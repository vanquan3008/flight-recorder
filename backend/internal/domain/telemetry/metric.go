package telemetry

import "time"

// MetricType identifies the OTLP instrument shape stored in a data point.
type MetricType string

const (
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeSum       MetricType = "sum"
	MetricTypeHistogram MetricType = "histogram"
)

// HistogramBucket is a single explicit-boundary bucket from an OTLP histogram.
type HistogramBucket struct {
	// UpperBound is +Inf for the last bucket.
	UpperBound float64 `json:"bound"`
	Count      uint64  `json:"count"`
}

// MetricDataPoint is the canonical domain representation of a single OTLP
// data point — one row per sample regardless of instrument type.
type MetricDataPoint struct {
	ID             string
	ProjectID      string
	EnvironmentID  string
	MetricName     string
	MetricUnit     string
	MetricType     MetricType
	ServiceName    string
	ServiceVersion string
	OccurredAt     time.Time

	// Gauge / Sum
	Value       *float64
	IsMonotonic *bool
	Temporality string

	// Histogram
	HistCount   *int64
	HistSum     *float64
	HistMin     *float64
	HistMax     *float64
	HistBuckets []HistogramBucket

	Attributes         map[string]any
	ResourceAttributes map[string]any
	ScopeName          string
	ScopeVersion       string
}
