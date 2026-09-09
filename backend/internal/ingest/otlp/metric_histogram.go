package otlp

import (
	"math"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func (n *normalizer) histogramPoints(base telemetry.MetricDataPoint, dps []*metricspb.HistogramDataPoint, resource *resourcepb.Resource, temporality string) ([]telemetry.MetricDataPoint, error) {
	base.MetricType = telemetry.MetricTypeHistogram
	base.Temporality = temporality
	points := make([]telemetry.MetricDataPoint, 0, len(dps))
	for _, dp := range dps {
		attrs, err := n.attributeValues(mergeAttributes(resource.GetAttributes(), dp.GetAttributes()))
		if err != nil {
			return points, err
		}
		ts := histogramDataPointTimestamp(dp)
		if ts.IsZero() {
			continue
		}
		point := base
		point.OccurredAt = ts
		point.Attributes = attrs
		count := int64(dp.GetCount())
		point.HistCount = &count
		if dp.Sum != nil {
			sum := dp.GetSum()
			point.HistSum = &sum
		}
		if dp.Min != nil {
			minimum := dp.GetMin()
			point.HistMin = &minimum
		}
		if dp.Max != nil {
			maximum := dp.GetMax()
			point.HistMax = &maximum
		}
		point.HistBuckets = buildBuckets(dp.GetExplicitBounds(), dp.GetBucketCounts())
		point.ID = metricPointID(point)
		points = append(points, point)
	}
	return points, nil
}

func (n *normalizer) expHistogramPoints(base telemetry.MetricDataPoint, dps []*metricspb.ExponentialHistogramDataPoint, resource *resourcepb.Resource, temporality string) ([]telemetry.MetricDataPoint, error) {
	base.MetricType = telemetry.MetricTypeHistogram
	base.Temporality = temporality
	points := make([]telemetry.MetricDataPoint, 0, len(dps))
	for _, dp := range dps {
		attrs, err := n.attributeValues(mergeAttributes(resource.GetAttributes(), dp.GetAttributes()))
		if err != nil {
			return points, err
		}
		ts := metricTimestamp(dp.GetTimeUnixNano())
		if ts.IsZero() {
			continue
		}
		point := base
		point.OccurredAt = ts
		point.Attributes = attrs
		count := int64(dp.GetCount())
		point.HistCount = &count
		if dp.Sum != nil {
			sum := dp.GetSum()
			point.HistSum = &sum
		}
		if dp.Min != nil {
			minimum := dp.GetMin()
			point.HistMin = &minimum
		}
		if dp.Max != nil {
			maximum := dp.GetMax()
			point.HistMax = &maximum
		}
		point.HistBuckets = []telemetry.HistogramBucket{
			{UpperBound: 0, Count: uint64(dp.GetZeroCount())},
			{UpperBound: math.Inf(1), Count: dp.GetCount()},
		}
		point.ID = metricPointID(point)
		points = append(points, point)
	}
	return points, nil
}

func (n *normalizer) summaryPoints(base telemetry.MetricDataPoint, dps []*metricspb.SummaryDataPoint, resource *resourcepb.Resource) ([]telemetry.MetricDataPoint, error) {
	base.MetricType = telemetry.MetricTypeHistogram
	points := make([]telemetry.MetricDataPoint, 0, len(dps))
	for _, dp := range dps {
		attrs, err := n.attributeValues(mergeAttributes(resource.GetAttributes(), dp.GetAttributes()))
		if err != nil {
			return points, err
		}
		ts := metricTimestamp(dp.GetTimeUnixNano())
		if ts.IsZero() {
			continue
		}
		point := base
		point.OccurredAt = ts
		point.Attributes = attrs
		count := int64(dp.GetCount())
		point.HistCount = &count
		sum := dp.GetSum()
		point.HistSum = &sum
		point.ID = metricPointID(point)
		points = append(points, point)
	}
	return points, nil
}

func buildBuckets(bounds []float64, counts []uint64) []telemetry.HistogramBucket {
	buckets := make([]telemetry.HistogramBucket, 0, len(counts))
	for index, count := range counts {
		bound := math.Inf(1)
		if index < len(bounds) {
			bound = bounds[index]
		}
		buckets = append(buckets, telemetry.HistogramBucket{UpperBound: bound, Count: count})
	}
	return buckets
}

func metricTimestamp(nanos uint64) time.Time {
	if nanos == 0 || nanos > maxSignedNanoseconds {
		return time.Time{}
	}
	return time.Unix(0, int64(nanos)).UTC()
}
