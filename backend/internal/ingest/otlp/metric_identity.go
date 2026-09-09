package otlp

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

func temporalityName(temporality metricspb.AggregationTemporality) string {
	if temporality == metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA {
		return "delta"
	}
	return "cumulative"
}

func numberDataPointTimestamp(point *metricspb.NumberDataPoint) time.Time {
	return metricTimestamp(point.GetTimeUnixNano())
}

func histogramDataPointTimestamp(point *metricspb.HistogramDataPoint) time.Time {
	return metricTimestamp(point.GetTimeUnixNano())
}

// metricPointID includes the complete series identity and sample payload, so
// retries remain idempotent without collapsing distinct samples.
func metricPointID(point telemetry.MetricDataPoint) string {
	identity := []string{
		"metric", point.ProjectID, point.EnvironmentID, point.MetricName,
		point.MetricUnit, string(point.MetricType), point.ServiceName,
		point.ServiceVersion, point.OccurredAt.Format(time.RFC3339Nano),
		point.Temporality, encodeAttrs(point.Attributes), encodeAttrs(point.ResourceAttributes),
		point.ScopeName, point.ScopeVersion,
	}
	if point.Value != nil {
		identity = append(identity, "value="+strconv.FormatFloat(*point.Value, 'g', -1, 64))
	}
	if point.IsMonotonic != nil {
		identity = append(identity, "monotonic="+strconv.FormatBool(*point.IsMonotonic))
	}
	if point.HistCount != nil {
		identity = append(identity, "count="+strconv.FormatInt(*point.HistCount, 10))
	}
	if point.HistSum != nil {
		identity = append(identity, "sum="+strconv.FormatFloat(*point.HistSum, 'g', -1, 64))
	}
	if point.HistMin != nil {
		identity = append(identity, "min="+strconv.FormatFloat(*point.HistMin, 'g', -1, 64))
	}
	if point.HistMax != nil {
		identity = append(identity, "max="+strconv.FormatFloat(*point.HistMax, 'g', -1, 64))
	}
	for _, bucket := range point.HistBuckets {
		identity = append(identity, "bucket="+strconv.FormatFloat(bucket.UpperBound, 'g', -1, 64)+":"+strconv.FormatUint(bucket.Count, 10))
	}
	return telemetry.DeterministicID(point.OccurredAt, identity...)
}

func encodeAttrs(attributes map[string]any) string {
	if len(attributes) == 0 {
		return ""
	}
	encoded, err := json.Marshal(attributes)
	if err != nil {
		return ""
	}
	return string(encoded)
}
