package telemetry

import "time"

type MetricType string

type MetricExemplar struct {
	OccurredAt time.Time      `json:"occurred_at"`
	Value      float64        `json:"value"`
	TraceID    string         `json:"trace_id,omitempty"`
	SpanID     string         `json:"span_id,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

const (
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeSum       MetricType = "sum"
	MetricTypeHistogram MetricType = "histogram"
)

type HistogramBucket struct {
	UpperBound float64 `json:"bound"`
	Count      uint64  `json:"count"`
}

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
	StartedAt      time.Time
	Flags          uint32

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
	ScopeAttributes    map[string]any
	Exemplars          []MetricExemplar
}
