// Copyright (c) 2023 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package tally

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

var (
	// alphanumericSanitizerOpts is the options to create a sanitizer which uses
	// the alphanumeric SanitizeFn.
	alphanumericSanitizerOpts = SanitizeOptions{
		NameCharacters: ValidCharacters{
			Ranges:     AlphanumericRange,
			Characters: UnderscoreDashCharacters,
		},
		KeyCharacters: ValidCharacters{
			Ranges:     AlphanumericRange,
			Characters: UnderscoreDashCharacters,
		},
		ValueCharacters: ValidCharacters{
			Ranges:     AlphanumericRange,
			Characters: UnderscoreDashCharacters,
		},
		ReplacementCharacter: DefaultReplacementCharacter,
	}
)

type testIntValue struct {
	val      int64
	tags     map[string]string
	reporter *testStatsReporter
}

func (m *testIntValue) ReportCount(value int64) {
	m.val = value
	m.reporter.cg.Done()
}

func (m *testIntValue) ReportTimer(interval time.Duration) {
	m.val = int64(interval)
	m.reporter.tg.Done()
}

type testFloatValue struct {
	val      float64
	tags     map[string]string
	reporter *testStatsReporter
}

func (m *testFloatValue) ReportGauge(value float64) {
	m.val = value
	m.reporter.gg.Done()
}

type testHistogramValue struct {
	tags            map[string]string
	valueSamples    map[float64]int
	durationSamples map[time.Duration]int
}

func newTestHistogramValue() *testHistogramValue {
	return &testHistogramValue{
		valueSamples:    make(map[float64]int),
		durationSamples: make(map[time.Duration]int),
	}
}

type testStatsReporter struct {
	cg sync.WaitGroup
	gg sync.WaitGroup
	tg sync.WaitGroup
	hg sync.WaitGroup

	counters   map[string]*testIntValue
	gauges     map[string]*testFloatValue
	timers     map[string]*testIntValue
	histograms map[string]*testHistogramValue

	flushes int32
}

// newTestStatsReporter returns a new TestStatsReporter
func newTestStatsReporter() *testStatsReporter {
	return &testStatsReporter{
		counters:   make(map[string]*testIntValue),
		gauges:     make(map[string]*testFloatValue),
		timers:     make(map[string]*testIntValue),
		histograms: make(map[string]*testHistogramValue),
	}
}

func (r *testStatsReporter) getCounters() map[string]*testIntValue {
	dst := make(map[string]*testIntValue, len(r.counters))
	for k, v := range r.counters {
		var (
			parts = strings.Split(k, "+")
			name  string
		)
		if len(parts) > 0 {
			name = parts[0]
		}

		dst[name] = v
	}

	return dst
}

func (r *testStatsReporter) getGauges() map[string]*testFloatValue {
	dst := make(map[string]*testFloatValue, len(r.gauges))
	for k, v := range r.gauges {
		var (
			parts = strings.Split(k, "+")
			name  string
		)
		if len(parts) > 0 {
			name = parts[0]
		}

		dst[name] = v
	}

	return dst
}

func (r *testStatsReporter) getTimers() map[string]*testIntValue {
	dst := make(map[string]*testIntValue, len(r.timers))
	for k, v := range r.timers {
		var (
			parts = strings.Split(k, "+")
			name  string
		)
		if len(parts) > 0 {
			name = parts[0]
		}

		dst[name] = v
	}

	return dst
}

func (r *testStatsReporter) getHistograms() map[string]*testHistogramValue {
	dst := make(map[string]*testHistogramValue, len(r.histograms))
	for k, v := range r.histograms {
		var (
			parts = strings.Split(k, "+")
			name  string
		)
		if len(parts) > 0 {
			name = parts[0]
		}

		dst[name] = v
	}

	return dst
}

func (r *testStatsReporter) WaitAll() {
	r.cg.Wait()
	r.gg.Wait()
	r.tg.Wait()
	r.hg.Wait()
}

func (r *testStatsReporter) AllocateCounter(
	name string, tags map[string]string,
) CachedCount {
	counter := &testIntValue{
		val:      0,
		tags:     tags,
		reporter: r,
	}
	r.counters[name] = counter
	return counter
}

func (r *testStatsReporter) ReportCounter(name string, tags map[string]string, value int64) {
	r.counters[name] = &testIntValue{
		val:  value,
		tags: tags,
	}
	r.cg.Done()
}

func (r *testStatsReporter) AllocateGauge(
	name string, tags map[string]string,
) CachedGauge {
	gauge := &testFloatValue{
		val:      0,
		tags:     tags,
		reporter: r,
	}
	r.gauges[name] = gauge
	return gauge
}

func (r *testStatsReporter) ReportGauge(name string, tags map[string]string, value float64) {
	r.gauges[name] = &testFloatValue{
		val:  value,
		tags: tags,
	}
	r.gg.Done()
}

func (r *testStatsReporter) AllocateTimer(
	name string, tags map[string]string,
) CachedTimer {
	timer := &testIntValue{
		val:      0,
		tags:     tags,
		reporter: r,
	}
	r.timers[name] = timer
	return timer
}

func (r *testStatsReporter) ReportTimer(name string, tags map[string]string, interval time.Duration) {
	r.timers[name] = &testIntValue{
		val:  int64(interval),
		tags: tags,
	}
	r.tg.Done()
}

func (r *testStatsReporter) AllocateHistogram(
	name string,
	tags map[string]string,
	buckets Buckets,
) CachedHistogram {
	return testStatsReporterCachedHistogram{r, name, tags, buckets}
}

type testStatsReporterCachedHistogram struct {
	r       *testStatsReporter
	name    string
	tags    map[string]string
	buckets Buckets
}

func (h testStatsReporterCachedHistogram) ValueBucket(
	bucketLowerBound, bucketUpperBound float64,
) CachedHistogramBucket {
	return testStatsReporterCachedHistogramValueBucket{h, bucketLowerBound, bucketUpperBound}
}

func (h testStatsReporterCachedHistogram) DurationBucket(
	bucketLowerBound, bucketUpperBound time.Duration,
) CachedHistogramBucket {
	return testStatsReporterCachedHistogramDurationBucket{h, bucketLowerBound, bucketUpperBound}
}

type testStatsReporterCachedHistogramValueBucket struct {
	histogram        testStatsReporterCachedHistogram
	bucketLowerBound float64
	bucketUpperBound float64
}

func (b testStatsReporterCachedHistogramValueBucket) ReportSamples(v int64) {
	b.histogram.r.ReportHistogramValueSamples(
		b.histogram.name, b.histogram.tags,
		b.histogram.buckets, b.bucketLowerBound, b.bucketUpperBound, v,
	)
}

type testStatsReporterCachedHistogramDurationBucket struct {
	histogram        testStatsReporterCachedHistogram
	bucketLowerBound time.Duration
	bucketUpperBound time.Duration
}

func (b testStatsReporterCachedHistogramDurationBucket) ReportSamples(v int64) {
	b.histogram.r.ReportHistogramDurationSamples(
		b.histogram.name, b.histogram.tags,
		b.histogram.buckets, b.bucketLowerBound, b.bucketUpperBound, v,
	)
}

func (r *testStatsReporter) ReportHistogramValueSamples(
	name string,
	tags map[string]string,
	buckets Buckets,
	bucketLowerBound float64,
	bucketUpperBound float64,
	samples int64,
) {
	key := KeyForPrefixedStringMap(name, tags)
	value, ok := r.histograms[key]
	if !ok {
		value = newTestHistogramValue()
		value.tags = tags
		r.histograms[key] = value
	}
	value.valueSamples[bucketUpperBound] = int(samples)
	r.hg.Done()
}

func (r *testStatsReporter) ReportHistogramDurationSamples(
	name string,
	tags map[string]string,
	buckets Buckets,
	bucketLowerBound time.Duration,
	bucketUpperBound time.Duration,
	samples int64,
) {
	key := KeyForPrefixedStringMap(name, tags)
	value, ok := r.histograms[key]
	if !ok {
		value = newTestHistogramValue()
		value.tags = tags
		r.histograms[key] = value
	}
	value.durationSamples[bucketUpperBound] = int(samples)
	r.hg.Done()
}

func (r *testStatsReporter) Capabilities() Capabilities {
	return capabilitiesReportingNoTagging
}

func (r *testStatsReporter) Flush() {
	atomic.AddInt32(&r.flushes, 1)
}

func TestWriteTimerImmediately(t *testing.T) {
	r := newTestStatsReporter()
	s, closer := NewRootScope(ScopeOptions{Reporter: r, MetricsOption: OmitInternalMetrics}, 0)
	defer closer.Close()
	r.tg.Add(1)
	s.Timer("ticky").Record(time.Millisecond * 175)
	r.tg.Wait()
}

func TestWriteTimerClosureImmediately(t *testing.T) {
	r := newTestStatsReporter()
	s, closer := NewRootScope(ScopeOptions{Reporter: r, MetricsOption: OmitInternalMetrics}, 0)
	defer closer.Close()
	r.tg.Add(1)
	tm := s.Timer("ticky")
	tm.Start().Stop()
	r.tg.Wait()
}

func TestWriteReportLoop(t *testing.T) {
	r := newTestStatsReporter()
	s, closer := NewRootScope(ScopeOptions{Reporter: r, MetricsOption: OmitInternalMetrics}, 10)
	defer closer.Close()

	r.cg.Add(1)
	s.Counter("bar").Inc(1)
	r.gg.Add(1)
	s.Gauge("zed").Update(1)
	r.tg.Add(1)
	s.Timer("ticky").Record(time.Millisecond * 175)
	r.hg.Add(1)
	s.Histogram("baz", MustMakeLinearValueBuckets(0, 10, 10)).
		RecordValue(42.42)

	r.WaitAll()
}

func TestWriteReportLoopDefaultInterval(t *testing.T) {
	r := newTestStatsReporter()
	s, closer := NewRootScopeWithDefaultInterval(
		ScopeOptions{Reporter: r, MetricsOption: OmitInternalMetrics},
	)
	defer closer.Close()

	r.cg.Add(1)
	s.Counter("bar").Inc(1)
	r.gg.Add(1)
	s.Gauge("zed").Update(1)
	r.tg.Add(1)
	s.Timer("ticky").Record(time.Millisecond * 175)
	r.hg.Add(1)
	s.Histogram("baz", MustMakeLinearValueBuckets(0, 10, 10)).
		RecordValue(42.42)

	r.WaitAll()
}

func TestCachedReportLoop(t *testing.T) {
	r := newTestStatsReporter()
	s, closer := NewRootScope(ScopeOptions{CachedReporter: r, MetricsOption: OmitInternalMetrics}, 10)
	defer closer.Close()

	r.cg.Add(1)
	s.Counter("bar").Inc(1)
	r.gg.Add(1)
	s.Gauge("zed").Update(1)
	r.tg.Add(1)
	s.Timer("ticky").Record(time.Millisecond * 175)
	r.hg.Add(1)
	s.Histogram("baz", MustMakeLinearValueBuckets(0, 10, 10)).
		RecordValue(42.42)
	r.WaitAll()
}

func testReportLoopFlushOnce(t *testing.T, cached bool) {
	r := newTestStatsReporter()

	scopeOpts := ScopeOptions{CachedReporter: r, MetricsOption: OmitInternalMetrics}
	if !cached {
		scopeOpts = ScopeOptions{Reporter: r, MetricsOption: OmitInternalMetrics}
	}

	s, closer := NewRootScope(scopeOpts, 10*time.Minute)

	r.cg.Add(2)
	s.Counter("foobar").Inc(1)
	s.SubScope("baz").Counter("bar").Inc(1)
	r.gg.Add(2)
	s.Gauge("zed").Update(1)
	s.SubScope("baz").Gauge("zed").Update(1)
	r.tg.Add(2)
	s.Timer("ticky").Record(time.Millisecond * 175)
	s.SubScope("woof").Timer("sod").Record(time.Millisecond * 175)
	r.hg.Add(2)
	s.SubScope("woofers").Histogram("boo", MustMakeLinearValueBuckets(0, 10, 10)).
		RecordValue(42.42)
	s.Histogram("baz", MustMakeLinearValueBuckets(0, 10, 10)).
		RecordValue(42.42)

	closer.Close()
	r.WaitAll()

	v := atomic.LoadInt32(&r.flushes)
	assert.Equal(t, int32(1), v)
}

func TestCachedReporterFlushOnce(t *testing.T) {
	testReportLoopFlushOnce(t, true)
}

func TestReporterFlushOnce(t *testing.T) {
	testReportLoopFlushOnce(t, false)
}

func TestWriteOnce(t *testing.T) {
	r := newTestStatsReporter()

	root, closer := NewRootScope(ScopeOptions{Reporter: r, MetricsOption: OmitInternalMetrics}, 0)
	defer closer.Close()

	s := root.(*scope)

	r.cg.Add(1)
	s.Counter("bar").Inc(1)
	r.gg.Add(1)
	s.Gauge("zed").Update(1)
	r.tg.Add(1)
	s.Timer("ticky").Record(time.Millisecond * 175)
	r.hg.Add(1)
	s.Histogram("baz", MustMakeLinearValueBuckets(0, 10, 10)).
		RecordValue(42.42)
	r.hg.Add(1)
	s.Histogram("bat", MustMakeLinearValueBuckets(1, 1, 3)).RecordValue(2.1)
	r.hg.Add(1)
	s.SubScope("test").Histogram("bat", MustMakeLinearValueBuckets(1, 1, 3)).RecordValue(1.1)
	r.hg.Add(1)
	s.SubScope("test").Histogram("bat", MustMakeLinearValueBuckets(1, 1, 3)).RecordValue(2.1)

	buckets := MustMakeLinearValueBuckets(100, 10, 3)
	r.hg.Add(1)
	s.SubScope("test").Histogram("qux", buckets).RecordValue(135.0)
	r.hg.Add(1)
	s.SubScope("test").Histogram("quux", buckets).RecordValue(101.0)
	r.hg.Add(1)
	s.SubScope("test2").Histogram("quux", buckets).RecordValue(101.0)

	s.reportLoopRun()

	r.WaitAll()

	var (
		counters   = r.getCounters()
		gauges     = r.getGauges()
		timers     = r.getTimers()
		histograms = r.getHistograms()
	)

	assert.EqualValues(t, 1, counters["bar"].val)
	assert.EqualValues(t, 1, gauges["zed"].val)
	assert.EqualValues(t, time.Millisecond*175, timers["ticky"].val)
	assert.EqualValues(t, 1, histograms["baz"].valueSamples[50.0])
	assert.EqualValues(t, 1, histograms["bat"].valueSamples[3.0])
	assert.EqualValues(t, 1, histograms["test.bat"].valueSamples[2.0])
	assert.EqualValues(t, 1, histograms["test.bat"].valueSamples[3.0])
	assert.EqualValues(t, 1, histograms["test.qux"].valueSamples[math.MaxFloat64])
	assert.EqualValues(t, 1, histograms["test.quux"].valueSamples[110.0])
	assert.EqualValues(t, 1, histograms["test2.quux"].valueSamples[110.0])

	r = newTestStatsReporter()
	s.reportLoopRun()

	counters = r.getCounters()
	gauges = r.getGauges()
	timers = r.getTimers()
	histograms = r.getHistograms()

	assert.Nil(t, counters["bar"])
	assert.Nil(t, gauges["zed"])
	assert.Nil(t, timers["ticky"])
	assert.Nil(t, histograms["baz"])
	assert.Nil(t, histograms["bat"])
	assert.Nil(t, histograms["test.qux"])
}

func TestHistogramSharedBucketMetrics(t *testing.T) {
	var (
		r     = newTestStatsReporter()
		scope = newRootScope(ScopeOptions{
			Prefix:         "",
			Tags:           nil,
			CachedReporter: r,
			MetricsOption:  OmitInternalMetrics,
		}, 0)
		builder = func(s Scope) func(map[string]string) {
			buckets := MustMakeLinearValueBuckets(10, 10, 3)
			return func(tags map[string]string) {
				s.Tagged(tags).Histogram("hist", buckets).RecordValue(19.0)
			}
		}
	)

	var (
		wg     = &sync.WaitGroup{}
		record = builder(scope)
	)

	r.hg.Add(4)
	for i := 0; i < 10000; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()

			val := strconv.Itoa(i % 4)
			record(
				map[string]string{
					"key": val,
				},
			)

			time.Sleep(time.Duration(rand.Float64() * float64(time.Second)))
		}()
	}

	wg.Wait()
	scope.reportRegistry()
	r.WaitAll()

	unseen := map[string]struct{}{
		"0": {},
		"1": {},
		"2": {},
		"3": {},
	}

	require.Equal(t, len(unseen), len(r.histograms))

	for name, value := range r.histograms {
		if !strings.HasPrefix(name, "hist+") {
			continue
		}

		count, ok := value.valueSamples[20.0]
		require.True(t, ok)
		require.Equal(t, 2500, count)

		delete(unseen, value.tags["key"])
	}

	require.Equal(t, 0, len(unseen), fmt.Sprintf("%v", unseen))
}

func TestConcurrentUpdates(t *testing.T) {
	var (
		r                = newTestStatsReporter()
		wg               = &sync.WaitGroup{}
		workerCount      = 20
		scopeCount       = 4
		countersPerScope = 4
		counterIncrs     = 5000
		rs               = newRootScope(
			ScopeOptions{
				Prefix:         "",
				Tags:           nil,
				CachedReporter: r,
				MetricsOption:  OmitInternalMetrics,
			}, 0,
		)
		scopes   = []Scope{rs}
		counters []Counter
	)

	// Instantiate Subscopes.
	for i := 1; i < scopeCount; i++ {
		scopes = append(scopes, rs.SubScope(fmt.Sprintf("subscope_%d", i)))
	}

	// Instantiate Counters.
	for sNum, s := range scopes {
		for cNum := 0; cNum < countersPerScope; cNum++ {
			counters = append(counters, s.Counter(fmt.Sprintf("scope_%d_counter_%d", sNum, cNum)))
		}
	}

	// Instantiate workers.
	r.cg.Add(scopeCount * countersPerScope)
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Counter should have counterIncrs * workerCount.
			for i := 0; i < counterIncrs*len(counters); i++ {
				counters[i%len(counters)].Inc(1)
			}
		}()
	}

	wg.Wait()
	rs.reportRegistry()
	r.WaitAll()

	wantVal := int64(workerCount * counterIncrs)
	for _, gotCounter := range r.getCounters() {
		assert.Equal(t, gotCounter.val, wantVal)
	}
}

func TestCounterSanitized(t *testing.T) {
	r := newTestStatsReporter()

	root, closer := NewRootScope(ScopeOptions{
		Reporter:        r,
		SanitizeOptions: &alphanumericSanitizerOpts,
		MetricsOption:   OmitInternalMetrics,
	}, 0)
	defer closer.Close()

	s := root.(*scope)

	r.cg.Add(1)
	s.Counter("how?").Inc(1)
	r.gg.Add(1)
	s.Gauge("does!").Update(1)
	r.tg.Add(1)
	s.Timer("this!").Record(time.Millisecond * 175)
	r.hg.Add(1)
	s.Histogram("work1!?", MustMakeLinearValueBuckets(0, 10, 10)).
		RecordValue(42.42)

	s.report(r)
	r.WaitAll()

	var (
		counters   = r.getCounters()
		gauges     = r.getGauges()
		timers     = r.getTimers()
		histograms = r.getHistograms()
	)

	assert.Nil(t, counters["how?"])
	assert.EqualValues(t, 1, counters["how_"].val)
	assert.Nil(t, gauges["does!"])
	assert.EqualValues(t, 1, gauges["does_"].val)
	assert.Nil(t, timers["this!"])
	assert.EqualValues(t, time.Millisecond*175, timers["this_"].val)
	assert.Nil(t, histograms["work1!?"])
	assert.EqualValues(t, 1, histograms["work1__"].valueSamples[50.0])

	r = newTestStatsReporter()
	s.report(r)

	counters = r.getCounters()
	gauges = r.getGauges()
	timers = r.getTimers()
	histograms = r.getHistograms()

	assert.Nil(t, counters["how?"])
	assert.Nil(t, counters["how_"])
	assert.Nil(t, gauges["does!"])
	assert.Nil(t, gauges["does_"])
	assert.Nil(t, timers["this!"])
	assert.Nil(t, timers["this_"])
	assert.Nil(t, histograms["work1!?"])
	assert.Nil(t, histograms["work1__"])
}

func TestCachedReporter(t *testing.T) {
	r := newTestStatsReporter()

	root, closer := NewRootScope(ScopeOptions{CachedReporter: r, MetricsOption: OmitInternalMetrics}, 0)
	defer closer.Close()

	s := root.(*scope)

	r.cg.Add(1)
	s.Counter("bar").Inc(1)
	r.gg.Add(1)
	s.Gauge("zed").Update(1)
	r.tg.Add(1)
	s.Timer("ticky").Record(time.Millisecond * 175)
	r.hg.Add(2)
	s.Histogram("baz", MustMakeLinearValueBuckets(0, 10, 10)).
		RecordValue(42.42)
	s.Histogram("qux", MustMakeLinearDurationBuckets(0, 10*time.Millisecond, 10)).
		RecordDuration(42 * time.Millisecond)

	s.cachedReport()
	r.WaitAll()

	var (
		counters   = r.getCounters()
		gauges     = r.getGauges()
		timers     = r.getTimers()
		histograms = r.getHistograms()
	)

	assert.EqualValues(t, 1, counters["bar"].val)
	assert.EqualValues(t, 1, gauges["zed"].val)
	assert.EqualValues(t, time.Millisecond*175, timers["ticky"].val)
	assert.EqualValues(t, 1, histograms["baz"].valueSamples[50.0])
	assert.EqualValues(t, 1, histograms["qux"].durationSamples[50*time.Millisecond])
}

func TestRootScopeWithoutPrefix(t *testing.T) {
	r := newTestStatsReporter()

	root, closer := NewRootScope(ScopeOptions{Reporter: r, MetricsOption: OmitInternalMetrics}, 0)
	defer closer.Close()

	s := root.(*scope)
	r.cg.Add(1)
	s.Counter("bar").Inc(1)
	s.Counter("bar").Inc(20)
	r.gg.Add(1)
	s.Gauge("zed").Update(1)
	r.tg.Add(1)
	s.Timer("blork").Record(time.Millisecond * 175)
	r.hg.Add(1)
	s.Histogram("baz", MustMakeLinearValueBuckets(0, 10, 10)).
		RecordValue(42.42)

	s.report(r)
	r.WaitAll()

	var (
		counters   = r.getCounters()
		gauges     = r.getGauges()
		timers     = r.getTimers()
		histograms = r.getHistograms()
	)

	assert.EqualValues(t, 21, counters["bar"].val)
	assert.EqualValues(t, 1, gauges["zed"].val)
	assert.EqualValues(t, time.Millisecond*175, timers["blork"].val)
	assert.EqualValues(t, 1, histograms["baz"].valueSamples[50.0])
}

func TestRootScopeWithPrefix(t *testing.T) {
	r := newTestStatsReporter()

	root, closer := NewRootScope(
		ScopeOptions{Prefix: "foo", Reporter: r, MetricsOption: OmitInternalMetrics}, 0,
	)
	defer closer.Close()

	s := root.(*scope)
	r.cg.Add(1)
	s.Counter("bar").Inc(1)
	s.Counter("bar").Inc(20)
	r.gg.Add(1)
	s.Gauge("zed").Update(1)
	r.tg.Add(1)
	s.Timer("blork").Record(time.Millisecond * 175)
	r.hg.Add(1)
	s.Histogram("baz", MustMakeLinearValueBuckets(0, 10, 10)).
		RecordValue(42.42)

	s.report(r)
	r.WaitAll()

	var (
		counters   = r.getCounters()
		gauges     = r.getGauges()
		timers     = r.getTimers()
		histograms = r.getHistograms()
	)

	assert.EqualValues(t, 21, counters["foo.bar"].val)
	assert.EqualValues(t, 1, gauges["foo.zed"].val)
	assert.EqualValues(t, time.Millisecond*175, timers["foo.blork"].val)
	assert.EqualValues(t, 1, histograms["foo.baz"].valueSamples[50.0])
}

func TestRootScopeWithDifferentSeparator(t *testing.T) {
	r := newTestStatsReporter()

	root, closer := NewRootScope(
		ScopeOptions{
			Prefix: "foo", Separator: "_", Reporter: r, MetricsOption: OmitInternalMetrics,
		}, 0,
	)
	defer closer.Close()

	s := root.(*scope)
	r.cg.Add(1)
	s.Counter("bar").Inc(1)
	s.Counter("bar").Inc(20)
	r.gg.Add(1)
	s.Gauge("zed").Update(1)
	r.tg.Add(1)
	s.Timer("blork").Record(time.Millisecond * 175)
	r.hg.Add(1)
	s.Histogram("baz", MustMakeLinearValueBuckets(0, 10, 10)).
		RecordValue(42.42)

	s.report(r)
	r.WaitAll()

	var (
		counters   = r.getCounters()
		gauges     = r.getGauges()
		timers     = r.getTimers()
		histograms = r.getHistograms()
	)

	assert.EqualValues(t, 21, counters["foo_bar"].val)
	assert.EqualValues(t, 1, gauges["foo_zed"].val)
	assert.EqualValues(t, time.Millisecond*175, timers["foo_blork"].val)
	assert.EqualValues(t, 1, histograms["foo_baz"].valueSamples[50.0])
}

func TestSubScope(t *testing.T) {
	r := newTestStatsReporter()

	root, closer := NewRootScope(
		ScopeOptions{Prefix: "foo", Reporter: r, MetricsOption: OmitInternalMetrics}, 0,
	)
	defer closer.Close()

	tags := map[string]string{"foo": "bar"}
	s := root.Tagged(tags).SubScope("mork").(*scope)
	r.cg.Add(1)
	s.Counter("bar").Inc(1)
	s.Counter("bar").Inc(20)
	r.gg.Add(1)
	s.Gauge("zed").Update(1)
	r.tg.Add(1)
	s.Timer("blork").Record(time.Millisecond * 175)
	r.hg.Add(1)
	s.Histogram("baz", MustMakeLinearValueBuckets(0, 10, 10)).
		RecordValue(42.42)

	s.report(r)
	r.WaitAll()

	var (
		counters   = r.getCounters()
		gauges     = r.getGauges()
		timers     = r.getTimers()
		histograms = r.getHistograms()
	)

	// Assert prefixed correctly
	assert.EqualValues(t, 21, counters["foo.mork.bar"].val)
	assert.EqualValues(t, 1, gauges["foo.mork.zed"].val)
	assert.EqualValues(t, time.Millisecond*175, timers["foo.mork.blork"].val)
	assert.EqualValues(t, 1, histograms["foo.mork.baz"].valueSamples[50.0])

	// Assert tags inherited
	assert.Equal(t, tags, counters["foo.mork.bar"].tags)
	assert.Equal(t, tags, gauges["foo.mork.zed"].tags)
	assert.Equal(t, tags, timers["foo.mork.blork"].tags)
	assert.Equal(t, tags, histograms["foo.mork.baz"].tags)
}

func TestSubScopeClose(t *testing.T) {
	r := newTestStatsReporter()

	rs, closer := NewRootScope(ScopeOptions{Prefix: "foo", Reporter: r, MetricsOption: OmitInternalMetrics}, 0)
	// defer closer.Close()
	_ = closer

	var (
		root        = rs.(*scope)
		s           = root.SubScope("mork").(*scope)
		rootCounter = root.Counter("foo")
		subCounter  = s.Counter("foo")
	)

	// Emit a metric from both scopes.
	r.cg.Add(1)
	rootCounter.Inc(1)
	r.cg.Add(1)
	subCounter.Inc(1)

	// Verify that we got both metrics.
	root.reportRegistry()
	r.WaitAll()
	counters := r.getCounters()
	require.EqualValues(t, 1, counters["foo.foo"].val)
	require.EqualValues(t, 1, counters["foo.mork.foo"].val)

	// Close the subscope. We expect both metrics to still be reported, because
	// we won't have reported the registry before we update the metrics.
	require.NoError(t, s.Close())

	// Create a subscope from the now-closed scope; it should nop.
	ns := s.SubScope("foobar")
	require.Equal(t, NoopScope, ns)

	// Emit a metric from all scopes.
	r.cg.Add(1)
	rootCounter.Inc(2)
	r.cg.Add(1)
	subCounter.Inc(2)

	// Verify that we still got both metrics.
	root.reportLoopRun()
	r.WaitAll()
	counters = r.getCounters()
	require.EqualValues(t, 2, counters["foo.foo"].val)
	require.EqualValues(t, 2, counters["foo.mork.foo"].val)

	// Emit a metric for both scopes. The root counter should succeed, and the
	// subscope counter should not update what's in the reporter.
	r.cg.Add(1)
	rootCounter.Inc(3)
	subCounter.Inc(3)
	root.reportLoopRun()
	r.WaitAll()
	time.Sleep(time.Second) // since we can't wg.Add the non-reported counter

	// We only expect the root scope counter; the subscope counter will be the
	// value previously held by the reporter, because it has not been udpated.
	counters = r.getCounters()
	require.EqualValues(t, 3, counters["foo.foo"].val)
	require.EqualValues(t, 2, counters["foo.mork.foo"].val)

	// Ensure that we can double-close harmlessly.
	require.NoError(t, s.Close())

	// Create one more scope so that we can ensure it's defunct once the root is
	// closed.
	ns = root.SubScope("newscope")

	// Close the root scope. We should not be able to emit any more metrics,
	// because the root scope reports the registry prior to closing.
	require.NoError(t, closer.Close())

	ns.Counter("newcounter").Inc(1)
	rootCounter.Inc(4)
	root.registry.Report(r)
	time.Sleep(time.Second) // since we can't wg.Add the non-reported counter

	// We do not expect any updates.
	counters = r.getCounters()
	require.EqualValues(t, 3, counters["foo.foo"].val)
	require.EqualValues(t, 2, counters["foo.mork.foo"].val)
	_, found := counters["newscope.newcounter"]
	require.False(t, found)

	// Ensure that we can double-close harmlessly.
	require.NoError(t, closer.Close())
}

func TestTaggedSubScope(t *testing.T) {
	r := newTestStatsReporter()

	ts := map[string]string{"env": "test"}
	root, closer := NewRootScope(
		ScopeOptions{
			Prefix: "foo", Tags: ts, Reporter: r, MetricsOption: OmitInternalMetrics,
		}, 0,
	)
	defer closer.Close()

	s := root.(*scope)

	tscope := root.Tagged(map[string]string{"service": "test"}).(*scope)
	scope := root

	r.cg.Add(1)
	scope.Counter("beep").Inc(1)
	r.cg.Add(1)
	tscope.Counter("boop").Inc(1)
	r.hg.Add(1)
	scope.Histogram("baz", MustMakeLinearValueBuckets(0, 10, 10)).
		RecordValue(42.42)
	r.hg.Add(1)
	tscope.Histogram("bar", MustMakeLinearValueBuckets(0, 10, 10)).
		RecordValue(42.42)

	s.report(r)
	tscope.report(r)
	r.cg.Wait()

	var (
		counters   = r.getCounters()
		histograms = r.getHistograms()
	)

	assert.EqualValues(t, 1, counters["foo.beep"].val)
	assert.EqualValues(t, ts, counters["foo.beep"].tags)

	assert.EqualValues(t, 1, counters["foo.boop"].val)
	assert.EqualValues(
		t, map[string]string{
			"env":     "test",
			"service": "test",
		}, counters["foo.boop"].tags,
	)

	assert.EqualValues(t, 1, histograms["foo.baz"].valueSamples[50.0])
	assert.EqualValues(t, ts, histograms["foo.baz"].tags)

	assert.EqualValues(t, 1, histograms["foo.bar"].valueSamples[50.0])
	assert.EqualValues(
		t, map[string]string{
			"env":     "test",
			"service": "test",
		}, histograms["foo.bar"].tags,
	)
}

func TestTaggedSanitizedSubScope(t *testing.T) {
	r := newTestStatsReporter()

	ts := map[string]string{"env": "test:env"}
	root, closer := NewRootScope(ScopeOptions{
		Prefix:          "foo",
		Tags:            ts,
		Reporter:        r,
		SanitizeOptions: &alphanumericSanitizerOpts,
		MetricsOption:   OmitInternalMetrics,
	}, 0)
	defer closer.Close()
	s := root.(*scope)

	tscope := root.Tagged(map[string]string{"service": "test.service"}).(*scope)

	r.cg.Add(1)
	tscope.Counter("beep").Inc(1)

	s.report(r)
	tscope.report(r)
	r.cg.Wait()

	counters := r.getCounters()
	assert.EqualValues(t, 1, counters["foo_beep"].val)
	assert.EqualValues(
		t, map[string]string{
			"env":     "test_env",
			"service": "test_service",
		}, counters["foo_beep"].tags,
	)
}

func TestTaggedExistingReturnsSameScope(t *testing.T) {
	r := newTestStatsReporter()

	for _, initialTags := range []map[string]string{
		nil,
		{"env": "test"},
	} {
		root, closer := NewRootScope(
			ScopeOptions{
				Prefix: "foo", Tags: initialTags, Reporter: r, MetricsOption: OmitInternalMetrics,
			}, 0,
		)
		defer closer.Close()

		rootScope := root.(*scope)
		fooScope := root.Tagged(map[string]string{"foo": "bar"}).(*scope)

		assert.NotEqual(t, rootScope, fooScope)
		assert.Equal(t, fooScope, fooScope.Tagged(nil))

		fooBarScope := fooScope.Tagged(map[string]string{"bar": "baz"}).(*scope)

		assert.NotEqual(t, fooScope, fooBarScope)
		assert.Equal(t, fooBarScope, fooScope.Tagged(map[string]string{"bar": "baz"}).(*scope))
	}
}

func TestSnapshot(t *testing.T) {
	commonTags := map[string]string{"env": "test"}
	s := NewTestScope("foo", map[string]string{"env": "test"})
	child := s.Tagged(map[string]string{"service": "test"})
	s.Counter("beep").Inc(1)
	s.Gauge("bzzt").Update(2)
	s.Timer("brrr").Record(1 * time.Second)
	s.Timer("brrr").Record(2 * time.Second)
	s.Histogram("fizz", ValueBuckets{0, 2, 4}).RecordValue(1)
	s.Histogram("fizz", ValueBuckets{0, 2, 4}).RecordValue(5)
	s.Histogram("buzz", DurationBuckets{time.Second * 2, time.Second * 4}).RecordDuration(time.Second)
	child.Counter("boop").Inc(1)

	// Should be able to call Snapshot any number of times and get same result.
	for i := 0; i < 3; i++ {
		t.Run(
			fmt.Sprintf("attempt %d", i), func(t *testing.T) {
				snap := s.Snapshot()
				counters, gauges, timers, histograms :=
					snap.Counters(), snap.Gauges(), snap.Timers(), snap.Histograms()

				assert.EqualValues(t, 1, counters["foo.beep+env=test"].Value())
				assert.EqualValues(t, commonTags, counters["foo.beep+env=test"].Tags())

				assert.EqualValues(t, 2, gauges["foo.bzzt+env=test"].Value())
				assert.EqualValues(t, commonTags, gauges["foo.bzzt+env=test"].Tags())

				assert.EqualValues(
					t, []time.Duration{
						1 * time.Second,
						2 * time.Second,
					}, timers["foo.brrr+env=test"].Values(),
				)
				assert.EqualValues(t, commonTags, timers["foo.brrr+env=test"].Tags())

				assert.EqualValues(
					t, map[float64]int64{
						0:               0,
						2:               1,
						4:               0,
						math.MaxFloat64: 1,
					}, histograms["foo.fizz+env=test"].Values(),
				)
				assert.EqualValues(t, map[time.Duration]int64(nil), histograms["foo.fizz+env=test"].Durations())
				assert.EqualValues(t, commonTags, histograms["foo.fizz+env=test"].Tags())

				assert.EqualValues(t, map[float64]int64(nil), histograms["foo.buzz+env=test"].Values())
				assert.EqualValues(
					t, map[time.Duration]int64{
						time.Second * 2: 1,
						time.Second * 4: 0,
						math.MaxInt64:   0,
					}, histograms["foo.buzz+env=test"].Durations(),
				)
				assert.EqualValues(t, commonTags, histograms["foo.buzz+env=test"].Tags())

				assert.EqualValues(t, 1, counters["foo.boop+env=test,service=test"].Value())
				assert.EqualValues(
					t, map[string]string{
						"env":     "test",
						"service": "test",
					}, counters["foo.boop+env=test,service=test"].Tags(),
				)
			},
		)
	}
}

func TestSnapshotConcurrent(t *testing.T) {
	var (
		scope = NewTestScope("", nil)
		quit  = make(chan struct{})
		done  = make(chan struct{})
	)

	go func() {
		defer close(done)
		for {
			select {
			case <-quit:
				return
			default:
				hello := scope.Tagged(map[string]string{"a": "b"}).Counter("hello")
				hello.Inc(1)
			}
		}
	}()
	var val CounterSnapshot
	for {
		val = scope.Snapshot().Counters()["hello+a=b"]
		if val != nil {
			quit <- struct{}{}
			break
		}
	}
	require.NotNil(t, val)

	<-done
}

func TestCapabilities(t *testing.T) {
	r := newTestStatsReporter()
	s, closer := NewRootScope(ScopeOptions{Reporter: r, MetricsOption: OmitInternalMetrics}, 0)
	defer closer.Close()
	assert.True(t, s.Capabilities().Reporting())
	assert.False(t, s.Capabilities().Tagging())
}

func TestCapabilitiesNoReporter(t *testing.T) {
	s, closer := NewRootScope(ScopeOptions{}, 0)
	defer closer.Close()
	assert.False(t, s.Capabilities().Reporting())
	assert.False(t, s.Capabilities().Tagging())
}

func TestNilTagMerge(t *testing.T) {
	assert.Nil(t, nil, mergeRightTags(nil, nil))
}

func TestScopeDefaultBuckets(t *testing.T) {
	r := newTestStatsReporter()

	root, closer := NewRootScope(ScopeOptions{
		DefaultBuckets: DurationBuckets{
			0 * time.Millisecond,
			30 * time.Millisecond,
			60 * time.Millisecond,
			90 * time.Millisecond,
			120 * time.Millisecond,
		},
		Reporter:      r,
		MetricsOption: OmitInternalMetrics,
	}, 0)
	defer closer.Close()

	s := root.(*scope)
	r.hg.Add(2)
	s.Histogram("baz", DefaultBuckets).RecordDuration(42 * time.Millisecond)
	s.Histogram("baz", DefaultBuckets).RecordDuration(84 * time.Millisecond)
	s.Histogram("baz", DefaultBuckets).RecordDuration(84 * time.Millisecond)

	s.report(r)
	r.WaitAll()

	histograms := r.getHistograms()
	assert.EqualValues(t, 1, histograms["baz"].durationSamples[60*time.Millisecond])
	assert.EqualValues(t, 2, histograms["baz"].durationSamples[90*time.Millisecond])
}

type testMets struct {
	c Counter
}

func newTestMets(scope Scope) testMets {
	return testMets{
		c: scope.Counter("honk"),
	}
}

func TestReturnByValue(t *testing.T) {
	r := newTestStatsReporter()

	root, closer := NewRootScope(ScopeOptions{Reporter: r, MetricsOption: OmitInternalMetrics}, 0)
	defer closer.Close()

	s := root.(*scope)
	mets := newTestMets(s)

	r.cg.Add(1)
	mets.c.Inc(3)
	s.report(r)
	r.cg.Wait()

	counters := r.getCounters()
	assert.EqualValues(t, 3, counters["honk"].val)
}

func TestScopeAvoidReportLoopRunOnClose(t *testing.T) {
	r := newTestStatsReporter()
	root, closer := NewRootScope(ScopeOptions{Reporter: r, MetricsOption: OmitInternalMetrics}, 0)

	s := root.(*scope)
	s.reportLoopRun()

	assert.Equal(t, int32(1), atomic.LoadInt32(&r.flushes))

	assert.NoError(t, closer.Close())

	s.reportLoopRun()
	assert.Equal(t, int32(2), atomic.LoadInt32(&r.flushes))
}

func TestScopeFlushOnClose(t *testing.T) {
	r := newTestStatsReporter()
	root, closer := NewRootScope(ScopeOptions{Reporter: r, MetricsOption: OmitInternalMetrics}, time.Hour)

	r.cg.Add(1)
	root.Counter("foo").Inc(1)

	counters := r.getCounters()
	assert.Nil(t, counters["foo"])
	assert.NoError(t, closer.Close())

	counters = r.getCounters()
	assert.EqualValues(t, 1, counters["foo"].val)
	assert.NoError(t, closer.Close())
}
