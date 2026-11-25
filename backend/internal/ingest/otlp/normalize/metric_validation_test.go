package normalize

import (
	"math"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/ingest/otlp/normalize/normalizetest"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func TestMetricsKeepValidSiblingsAndCountEveryRejectedPoint(t *testing.T) {
	now := uint64(time.Now().UnixNano())
	sum := 3.0
	for _, metric := range []*metricspb.Metric{
		{Name: "gauge", Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{numberPoint(now, 1), numberPoint(0, 2), numberPoint(now, 3)}}}},
		{Name: "sum", Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{AggregationTemporality: 2, IsMonotonic: true, DataPoints: []*metricspb.NumberDataPoint{numberPoint(now, 1), numberPoint(now, -1), numberPoint(now, 3)}}}},
		{Name: "hist", Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{AggregationTemporality: 2, DataPoints: []*metricspb.HistogramDataPoint{
			{TimeUnixNano: now, Count: 3, Sum: &sum, ExplicitBounds: []float64{1}, BucketCounts: []uint64{1, 2}},
			{TimeUnixNano: now, Count: 3, ExplicitBounds: []float64{1}, BucketCounts: []uint64{1}},
			{TimeUnixNano: now, Count: 3, Sum: &sum},
		}}}},
		{Name: "exponential", Data: &metricspb.Metric_ExponentialHistogram{ExponentialHistogram: &metricspb.ExponentialHistogram{AggregationTemporality: 2, DataPoints: []*metricspb.ExponentialHistogramDataPoint{
			{TimeUnixNano: now, Count: 3, Sum: &sum, ZeroCount: 3},
			{TimeUnixNano: now, Count: 3, ZeroCount: 1},
			{TimeUnixNano: now, Count: 3, Sum: &sum, Positive: &metricspb.ExponentialHistogramDataPoint_Buckets{BucketCounts: []uint64{3}}},
		}}}},
		{Name: "summary", Data: &metricspb.Metric_Summary{Summary: &metricspb.Summary{DataPoints: []*metricspb.SummaryDataPoint{
			{TimeUnixNano: now, Count: 3, Sum: sum}, {TimeUnixNano: now, Sum: math.NaN()}, {TimeUnixNano: now, Count: 2, Sum: sum},
		}}}},
	} {
		t.Run(metric.Name, func(t *testing.T) {
			points, rejected := New(normalizetest.Config()).Metrics(metricRequest(metric))
			if len(points) != 2 || rejected != 1 {
				t.Fatalf("accepted=%d rejected=%d, want 2/1", len(points), rejected)
			}
		})
	}
}

func TestMetricRequestBudgetIncludesInvalidInputs(t *testing.T) {
	now := uint64(time.Now().UnixNano())
	configuration := normalizetest.Config()
	configuration.MaxRecordsPerRequest = 2
	metric := &metricspb.Metric{Name: "gauge", Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{numberPoint(0, 1), numberPoint(now, 2), numberPoint(now, 3)}}}}
	points, rejected := New(configuration).Metrics(metricRequest(metric))
	if len(points) != 1 || rejected != 2 {
		t.Fatalf("accepted=%d rejected=%d", len(points), rejected)
	}
	request := metricRequest(metric)
	request[0].Resource.Attributes = nil // All three samples have no service identity.
	points, rejected = New(configuration).Metrics(request)
	if len(points) != 0 || rejected != 3 {
		t.Fatalf("missing resource identity: accepted=%d rejected=%d", len(points), rejected)
	}
}

func TestMetricMetadataAndInvalidValues(t *testing.T) {
	now := uint64(time.Now().UnixNano())
	valid := numberPoint(now, 1)
	valid.StartTimeUnixNano = now - uint64(time.Minute)
	noValue, reversed := numberPoint(now, 2), numberPoint(now, 3)
	noValue.Flags = 1
	reversed.StartTimeUnixNano = now + 1
	metric := &metricspb.Metric{Name: "gauge", Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{valid, noValue, reversed, numberPoint(now, math.Inf(1)), numberPoint(math.MaxUint64, 1)}}}}
	request := metricRequest(metric)
	request[0].ScopeMetrics[0].Scope = &commonpb.InstrumentationScope{Attributes: []*commonpb.KeyValue{normalizetest.StringKeyValue("scope.label", "one")}}
	points, rejected := New(normalizetest.Config()).Metrics(request)
	if len(points) != 1 || rejected != 4 {
		t.Fatalf("accepted=%d rejected=%d", len(points), rejected)
	}
	if points[0].StartedAt.IsZero() || points[0].ScopeAttributes["scope.label"] != "one" {
		t.Fatal("lost sample metadata")
	}
	request[0].ScopeMetrics[0].Scope.Attributes[0] = normalizetest.StringKeyValue("scope.label", "two")
	other, _ := New(normalizetest.Config()).Metrics(request)
	if points[0].ID == other[0].ID {
		t.Fatal("scope dimensions collapsed two samples")
	}
}

func TestExponentialBucketsKeepCountAndOrdering(t *testing.T) {
	dp := &metricspb.ExponentialHistogramDataPoint{Count: 10, ZeroCount: 2,
		Negative: &metricspb.ExponentialHistogramDataPoint_Buckets{Offset: 0, BucketCounts: []uint64{1, 2}},
		Positive: &metricspb.ExponentialHistogramDataPoint_Buckets{Offset: 0, BucketCounts: []uint64{2, 3}},
	}
	buckets, err := exponentialBuckets(dp)
	if err != nil {
		t.Fatal(err)
	}
	var count uint64
	for i, bucket := range buckets {
		count += bucket.Count
		if i > 0 && bucket.UpperBound <= buckets[i-1].UpperBound {
			t.Fatal("unordered bucket bounds")
		}
	}
	if count != dp.Count || !math.IsInf(buckets[len(buckets)-1].UpperBound, 1) {
		t.Fatalf("invalid buckets: %#v", buckets)
	}
}

func numberPoint(at uint64, value float64) *metricspb.NumberDataPoint {
	return &metricspb.NumberDataPoint{TimeUnixNano: at, Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: value}}
}

func metricRequest(metric *metricspb.Metric) []*metricspb.ResourceMetrics {
	return []*metricspb.ResourceMetrics{{Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{normalizetest.StringKeyValue("service.name", "checkout")}}, ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{metric}}}}}
}
