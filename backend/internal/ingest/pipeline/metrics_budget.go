package pipeline

import "github.com/vanquan3008/tracepilot/internal/domain/telemetry"

// estimateMetricBytes uses the event path's serialized-size estimate plus
// fixed overhead. Histogram +Inf bounds must stay outside the JSON encoder.
func estimateMetricBytes(point telemetry.MetricDataPoint) int64 {
	bucketBytes := int64(len(point.HistBuckets)) * 16
	point.HistBuckets = nil
	return 1024 + bucketBytes + int64(len(point.Exemplars))*128 + estimateBytes(point)
}
