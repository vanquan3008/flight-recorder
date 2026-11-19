package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

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

// SeriesID hashes everything that names the point's series, never values or times.
func (point MetricDataPoint) SeriesID() string {
	identity, _ := json.Marshal([]any{
		point.ServiceName, point.ServiceVersion, point.MetricName, point.MetricUnit, point.MetricType,
		point.Temporality, point.IsMonotonic, objectOrEmpty(point.Attributes), objectOrEmpty(point.ResourceAttributes),
		point.ScopeName, point.ScopeVersion, objectOrEmpty(point.ScopeAttributes),
	})
	sum := sha256.Sum256(identity)
	return hex.EncodeToString(sum[:16])
}

func objectOrEmpty(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}
