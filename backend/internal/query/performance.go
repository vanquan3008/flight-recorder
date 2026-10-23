package query

import (
	"errors"
	"time"
)

type PerformanceFilter struct {
	EnvironmentID string
	Service       string
	Endpoint      string
	From          time.Time
	To            time.Time
	Limit         int
}

func (filter PerformanceFilter) Validate() error {
	if filter.From.IsZero() || filter.To.IsZero() || !filter.From.Before(filter.To) || filter.To.Sub(filter.From) > 30*24*time.Hour {
		return errors.New("performance window must be positive and not exceed 30 days")
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		return errors.New("performance limit must be between 1 and 100")
	}
	if len(filter.Service) > 4096 || len(filter.Endpoint) > 4096 {
		return errors.New("service and endpoint filters must not exceed 4096 bytes")
	}
	return nil
}

type PerformanceStats struct {
	Requests            int64   `json:"requests"`
	Errors              int64   `json:"errors"`
	RequestsPerSecond   float64 `json:"requests_per_second"`
	ErrorRate           float64 `json:"error_rate"`
	AverageLatencyNanos int64   `json:"average_latency_nanos"`
	P95LatencyNanos     int64   `json:"p95_latency_nanos"`
	P99LatencyNanos     int64   `json:"p99_latency_nanos"`
}

func (stats *PerformanceStats) SetRates(seconds float64) {
	stats.RequestsPerSecond = float64(stats.Requests) / seconds
	if stats.Requests > 0 {
		stats.ErrorRate = float64(stats.Errors) / float64(stats.Requests)
	}
}

type PerformanceBucket struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
	PerformanceStats
}

type PerformanceGroup struct {
	Service  string `json:"service"`
	Endpoint string `json:"endpoint,omitempty"`
	PerformanceStats
}

type RequestPerformance struct {
	From          time.Time           `json:"from"`
	To            time.Time           `json:"to"`
	Service       string              `json:"service"`
	Endpoint      string              `json:"endpoint"`
	BucketSeconds int                 `json:"bucket_seconds"`
	Limit         int                 `json:"limit"`
	Summary       PerformanceStats    `json:"summary"`
	Series        []PerformanceBucket `json:"series"`
	Services      []PerformanceGroup  `json:"services"`
	Endpoints     []PerformanceGroup  `json:"endpoints"`
}
