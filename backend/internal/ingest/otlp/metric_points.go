package otlp

import (
	"errors"
	"math"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

// Normalize each point independently and apply admission before doing its work.
func collectMetricPoints[T any](raw []*T, limit int, normalize func(*T) (telemetry.MetricDataPoint, error)) ([]telemetry.MetricDataPoint, int) {
	points := make([]telemetry.MetricDataPoint, 0, min(len(raw), max(limit, 0)))
	rejected := 0
	for index, value := range raw {
		if index >= limit {
			rejected++
			continue
		}
		point, err := normalize(value)
		if err != nil {
			rejected++
			continue
		}
		point.ID = metricPointID(point)
		points = append(points, point)
	}
	return points, rejected
}

func metricPointCount(metric *metricspb.Metric) int {
	switch data := metric.GetData().(type) {
	case *metricspb.Metric_Gauge:
		return len(data.Gauge.GetDataPoints())
	case *metricspb.Metric_Sum:
		return len(data.Sum.GetDataPoints())
	case *metricspb.Metric_Histogram:
		return len(data.Histogram.GetDataPoints())
	case *metricspb.Metric_ExponentialHistogram:
		return len(data.ExponentialHistogram.GetDataPoints())
	case *metricspb.Metric_Summary:
		return len(data.Summary.GetDataPoints())
	default:
		return 0
	}
}

func (n *normalizer) metricPoint(base telemetry.MetricDataPoint, resource *resourcepb.Resource, attributes []*commonpb.KeyValue, start, end uint64, flags uint32, exemplars []*metricspb.Exemplar) (telemetry.MetricDataPoint, error) {
	if end == 0 || end > maxSignedNanoseconds || start > end || flags&1 != 0 {
		return telemetry.MetricDataPoint{}, errors.New("metric timestamp or recorded-value flag is invalid")
	}
	point := base
	point.OccurredAt = metricTimestamp(end)
	point.StartedAt = metricTimestamp(start)
	point.Flags = flags
	var err error
	point.Attributes, err = n.attributeValues(mergeAttributes(resource.GetAttributes(), attributes))
	if err != nil {
		return telemetry.MetricDataPoint{}, err
	}
	point.Exemplars, err = n.metricExemplars(exemplars, point.OccurredAt)
	return point, err
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func histogramValues(point *telemetry.MetricDataPoint, count uint64, sum, minimum, maximum *float64) error {
	if count > math.MaxInt64 || (sum != nil && !finite(*sum)) || (minimum != nil && !finite(*minimum)) || (maximum != nil && !finite(*maximum)) || (minimum != nil && maximum != nil && *minimum > *maximum) {
		return errors.New("histogram aggregates are invalid")
	}
	signedCount := int64(count)
	point.HistCount, point.HistSum, point.HistMin, point.HistMax = &signedCount, sum, minimum, maximum
	return nil
}
