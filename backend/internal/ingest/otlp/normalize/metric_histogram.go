package normalize

import (
	"errors"
	"math"
	"time"

	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func (n *Normalizer) histogramPoints(base telemetry.MetricDataPoint, raw []*metricspb.HistogramDataPoint, resource *resourcepb.Resource, limit int) ([]telemetry.MetricDataPoint, int) {
	return collectMetricPoints(raw, limit, func(dp *metricspb.HistogramDataPoint) (telemetry.MetricDataPoint, error) {
		point, err := n.metricPoint(base, resource, dp.GetAttributes(), dp.GetStartTimeUnixNano(), dp.GetTimeUnixNano(), dp.GetFlags(), dp.GetExemplars())
		if err != nil {
			return point, err
		}
		if err := histogramValues(&point, dp.GetCount(), dp.Sum, dp.Min, dp.Max); err != nil {
			return point, err
		}
		bounds, counts := dp.GetExplicitBounds(), dp.GetBucketCounts()
		if len(counts) > 0 && len(counts) != len(bounds)+1 {
			return point, errors.New("histogram bucket shape is invalid")
		}
		for index, bound := range bounds {
			if !finite(bound) || (index > 0 && bound <= bounds[index-1]) {
				return point, errors.New("histogram bounds are invalid")
			}
		}
		if len(counts) > 0 && !validBucketCount(dp.GetCount(), counts) {
			return point, errors.New("histogram bucket count is invalid")
		}
		if len(counts) == 0 && len(bounds) != 0 {
			return point, errors.New("histogram buckets are missing")
		}
		point.HistBuckets = buildBuckets(bounds, counts)
		return point, nil
	})
}

func (n *Normalizer) expHistogramPoints(base telemetry.MetricDataPoint, raw []*metricspb.ExponentialHistogramDataPoint, resource *resourcepb.Resource, limit int) ([]telemetry.MetricDataPoint, int) {
	return collectMetricPoints(raw, limit, func(dp *metricspb.ExponentialHistogramDataPoint) (telemetry.MetricDataPoint, error) {
		point, err := n.metricPoint(base, resource, dp.GetAttributes(), dp.GetStartTimeUnixNano(), dp.GetTimeUnixNano(), dp.GetFlags(), dp.GetExemplars())
		if err != nil {
			return point, err
		}
		if err := histogramValues(&point, dp.GetCount(), dp.Sum, dp.Min, dp.Max); err != nil {
			return point, err
		}
		point.HistBuckets, err = exponentialBuckets(dp)
		return point, err
	})
}

func exponentialBuckets(dp *metricspb.ExponentialHistogramDataPoint) ([]telemetry.HistogramBucket, error) {
	if dp.GetScale() < -10 || dp.GetScale() > 20 || !finite(dp.GetZeroThreshold()) || dp.GetZeroThreshold() < 0 {
		return nil, errors.New("exponential histogram scale or zero threshold is invalid")
	}
	negative, positive := dp.GetNegative(), dp.GetPositive()
	counts := append([]uint64{dp.GetZeroCount()}, negative.GetBucketCounts()...)
	counts = append(counts, positive.GetBucketCounts()...)
	if !validBucketCount(dp.GetCount(), counts) {
		return nil, errors.New("exponential histogram count is invalid")
	}
	buckets := make([]telemetry.HistogramBucket, 0, len(counts)+1)
	step := math.Exp2(-float64(dp.GetScale()))
	for index := len(negative.GetBucketCounts()) - 1; index >= 0; index-- {
		bound := -math.Exp2((float64(negative.GetOffset()) + float64(index)) * step)
		if !finite(bound) {
			return nil, errors.New("exponential histogram bound is invalid")
		}
		buckets = append(buckets, telemetry.HistogramBucket{UpperBound: bound, Count: negative.GetBucketCounts()[index]})
	}
	buckets = append(buckets, telemetry.HistogramBucket{UpperBound: dp.GetZeroThreshold(), Count: dp.GetZeroCount()})
	for index, count := range positive.GetBucketCounts() {
		bound := math.Exp2((float64(positive.GetOffset()) + float64(index) + 1) * step)
		if !finite(bound) {
			return nil, errors.New("exponential histogram bound is invalid")
		}
		buckets = append(buckets, telemetry.HistogramBucket{UpperBound: bound, Count: count})
	}
	return append(buckets, telemetry.HistogramBucket{UpperBound: math.Inf(1), Count: 0}), nil
}

func (n *Normalizer) summaryPoints(base telemetry.MetricDataPoint, raw []*metricspb.SummaryDataPoint, resource *resourcepb.Resource, limit int) ([]telemetry.MetricDataPoint, int) {
	return collectMetricPoints(raw, limit, func(dp *metricspb.SummaryDataPoint) (telemetry.MetricDataPoint, error) {
		point, err := n.metricPoint(base, resource, dp.GetAttributes(), dp.GetStartTimeUnixNano(), dp.GetTimeUnixNano(), dp.GetFlags(), nil)
		if err != nil {
			return point, err
		}
		sum := dp.GetSum()
		return point, histogramValues(&point, dp.GetCount(), &sum, nil, nil)
	})
}

func validBucketCount(total uint64, counts []uint64) bool {
	for _, count := range counts {
		if count > total {
			return false
		}
		total -= count
	}
	return total == 0
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
