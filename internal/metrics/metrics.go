package metrics

import (
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Registry struct {
	prometheusRegistry *prometheus.Registry
	httpRequests       *prometheus.CounterVec
	databaseReady      prometheus.Gauge
	ingestedRecords    *prometheus.CounterVec
	rejectedRecords    *prometheus.CounterVec
	ingestionQueue     prometheus.Gauge
	ingestionBytes     prometheus.Gauge
	batchDuration      prometheus.Histogram
	storageErrors      prometheus.Counter
	otlpRequests       *prometheus.CounterVec
	otlpDuration       *prometheus.HistogramVec
}

func New(version, commit, buildTime string) *Registry {
	prometheusRegistry := prometheus.NewRegistry()
	httpRequests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "flight",
		Name:      "http_requests_total",
		Help:      "Management HTTP requests completed.",
	}, []string{"method", "route", "status"})
	databaseReady := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "flight",
		Name:      "database_ready",
		Help:      "Whether PostgreSQL passed the latest readiness check.",
	})
	ingestedRecords := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "flight",
		Name:      "ingested_records_total",
		Help:      "OTLP records committed to PostgreSQL.",
	}, []string{"signal"})
	rejectedRecords := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "flight",
		Name:      "rejected_records_total",
		Help:      "OTLP records rejected before persistence.",
	}, []string{"signal", "reason"})
	ingestionQueue := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "flight",
		Name:      "ingestion_queue_depth",
		Help:      "Events currently waiting in the bounded ingestion queue.",
	})
	ingestionBytes := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "flight",
		Name:      "ingestion_inflight_bytes",
		Help:      "Estimated bytes admitted but not yet acknowledged by event storage.",
	})
	batchDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "flight",
		Name:      "event_batch_duration_seconds",
		Help:      "PostgreSQL event batch commit latency.",
		Buckets:   prometheus.DefBuckets,
	})
	storageErrors := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "flight",
		Name:      "event_storage_errors_total",
		Help:      "Event batch persistence failures after bounded retries.",
	})
	otlpRequests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "flight",
		Name:      "otlp_requests_total",
		Help:      "OTLP export requests completed.",
	}, []string{"transport", "signal", "outcome"})
	otlpDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "flight",
		Name:      "otlp_request_duration_seconds",
		Help:      "OTLP export request duration.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"transport", "signal"})
	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "flight",
		Name:      "build_info",
		Help:      "Flight build and runtime information.",
	}, []string{"version", "commit", "build_time", "go_version"})

	prometheusRegistry.MustRegister(httpRequests, databaseReady, ingestedRecords, rejectedRecords, ingestionQueue, ingestionBytes, batchDuration, storageErrors, otlpRequests, otlpDuration, buildInfo)
	buildInfo.WithLabelValues(version, commit, buildTime, runtime.Version()).Set(1)

	return &Registry{
		prometheusRegistry: prometheusRegistry,
		httpRequests:       httpRequests,
		databaseReady:      databaseReady,
		ingestedRecords:    ingestedRecords,
		rejectedRecords:    rejectedRecords,
		ingestionQueue:     ingestionQueue,
		ingestionBytes:     ingestionBytes,
		batchDuration:      batchDuration,
		storageErrors:      storageErrors,
		otlpRequests:       otlpRequests,
		otlpDuration:       otlpDuration,
	}
}

func (registry *Registry) ObserveOTLPRequest(transport, signal, outcome string, duration time.Duration) {
	registry.otlpRequests.WithLabelValues(transport, signal, outcome).Inc()
	registry.otlpDuration.WithLabelValues(transport, signal).Observe(duration.Seconds())
}

func (registry *Registry) ObserveIngested(signal string, count int) {
	registry.ingestedRecords.WithLabelValues(signal).Add(float64(count))
}

func (registry *Registry) ObserveRejected(signal, reason string, count int) {
	registry.rejectedRecords.WithLabelValues(signal, reason).Add(float64(count))
}

func (registry *Registry) SetIngestionQueueDepth(depth int) {
	registry.ingestionQueue.Set(float64(depth))
}

func (registry *Registry) SetIngestionBytes(bytes int64) {
	registry.ingestionBytes.Set(float64(bytes))
}

func (registry *Registry) ObserveBatch(duration time.Duration, failed bool) {
	registry.batchDuration.Observe(duration.Seconds())
	if failed {
		registry.storageErrors.Inc()
	}
}

func (registry *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(registry.prometheusRegistry, promhttp.HandlerOpts{})
}

func (registry *Registry) ObserveHTTPRequest(method, route string, statusCode int) {
	registry.httpRequests.WithLabelValues(method, route, strconv.Itoa(statusCode)).Inc()
}

func (registry *Registry) SetDatabaseReady(ready bool) {
	if ready {
		registry.databaseReady.Set(1)
		return
	}
	registry.databaseReady.Set(0)
}
