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
	httpDuration       *prometheus.HistogramVec
	databaseReady      prometheus.Gauge
	ingestedRecords    *prometheus.CounterVec
	rejectedRecords    *prometheus.CounterVec
	ingestionQueue     prometheus.Gauge
	ingestionBytes     prometheus.Gauge
	batchDuration      prometheus.Histogram
	storageErrors      prometheus.Counter
	ingestionRetries   prometheus.Counter
	admissionTimeouts  prometheus.Counter
	otlpRequests       *prometheus.CounterVec
	otlpDuration       *prometheus.HistogramVec
	retentionDeleted   prometheus.Counter
	retentionBacklog   prometheus.Gauge
	retentionErrors    prometheus.Counter
	retentionDuration  prometheus.Histogram
}

func New(version, commit, buildTime string) *Registry {
	prometheusRegistry := prometheus.NewRegistry()
	httpRequests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tracepilot",
		Name:      "http_requests_total",
		Help:      "Management HTTP requests completed.",
	}, []string{"method", "route", "status"})
	httpDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "tracepilot",
		Name:      "http_request_duration_seconds",
		Help:      "Management HTTP request duration.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "route"})
	databaseReady := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "tracepilot",
		Name:      "database_ready",
		Help:      "Whether PostgreSQL passed the latest readiness check.",
	})
	ingestedRecords := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tracepilot",
		Name:      "ingested_records_total",
		Help:      "OTLP records committed to PostgreSQL.",
	}, []string{"signal"})
	rejectedRecords := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tracepilot",
		Name:      "rejected_records_total",
		Help:      "OTLP records rejected before persistence.",
	}, []string{"signal", "reason"})
	ingestionQueue := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "tracepilot",
		Name:      "ingestion_queue_depth",
		Help:      "Events currently waiting in the bounded ingestion queue.",
	})
	ingestionBytes := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "tracepilot",
		Name:      "ingestion_inflight_bytes",
		Help:      "Estimated bytes admitted but not yet acknowledged by event storage.",
	})
	batchDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "tracepilot",
		Name:      "event_batch_duration_seconds",
		Help:      "PostgreSQL event batch commit latency.",
		Buckets:   prometheus.DefBuckets,
	})
	storageErrors := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "tracepilot",
		Name:      "event_storage_errors_total",
		Help:      "Event batch persistence failures after bounded retries.",
	})
	ingestionRetries := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "tracepilot",
		Name:      "ingestion_retries_total",
		Help:      "Retries attempted for failed event batch persistence.",
	})
	admissionTimeouts := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "tracepilot",
		Name:      "ingestion_admission_timeouts_total",
		Help:      "Ingestion submissions that exceeded the bounded admission timeout.",
	})
	otlpRequests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tracepilot",
		Name:      "otlp_requests_total",
		Help:      "OTLP export requests completed.",
	}, []string{"transport", "signal", "outcome"})
	otlpDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "tracepilot",
		Name:      "otlp_request_duration_seconds",
		Help:      "OTLP export request duration.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"transport", "signal"})
	retentionDeleted := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "tracepilot",
		Name:      "retention_deleted_events_total",
		Help:      "Events deleted by retention cleanup.",
	})
	retentionBacklog := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "tracepilot",
		Name:      "retention_backlog_events",
		Help:      "Events that remain older than the configured retention cutoff after cleanup.",
	})
	retentionErrors := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "tracepilot",
		Name:      "retention_errors_total",
		Help:      "Retention cleanup failures.",
	})
	retentionDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "tracepilot",
		Name:      "retention_duration_seconds",
		Help:      "Retention cleanup duration.",
		Buckets:   prometheus.DefBuckets,
	})
	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "tracepilot",
		Name:      "build_info",
		Help:      "TracePilot build and runtime information.",
	}, []string{"version", "commit", "build_time", "go_version"})

	prometheusRegistry.MustRegister(httpRequests, httpDuration, databaseReady, ingestedRecords, rejectedRecords, ingestionQueue, ingestionBytes, batchDuration, storageErrors, ingestionRetries, admissionTimeouts, otlpRequests, otlpDuration, retentionDeleted, retentionBacklog, retentionErrors, retentionDuration, buildInfo)
	buildInfo.WithLabelValues(version, commit, buildTime, runtime.Version()).Set(1)

	return &Registry{
		prometheusRegistry: prometheusRegistry,
		httpRequests:       httpRequests,
		httpDuration:       httpDuration,
		databaseReady:      databaseReady,
		ingestedRecords:    ingestedRecords,
		rejectedRecords:    rejectedRecords,
		ingestionQueue:     ingestionQueue,
		ingestionBytes:     ingestionBytes,
		batchDuration:      batchDuration,
		storageErrors:      storageErrors,
		ingestionRetries:   ingestionRetries,
		admissionTimeouts:  admissionTimeouts,
		otlpRequests:       otlpRequests,
		otlpDuration:       otlpDuration,
		retentionDeleted:   retentionDeleted,
		retentionBacklog:   retentionBacklog,
		retentionErrors:    retentionErrors,
		retentionDuration:  retentionDuration,
	}
}

func (registry *Registry) SetRetentionBacklog(count int64) {
	if count < 0 {
		count = 0
	}
	registry.retentionBacklog.Set(float64(count))
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

func (registry *Registry) ObserveRetry() {
	registry.ingestionRetries.Inc()
}

func (registry *Registry) ObserveAdmissionTimeout() {
	registry.admissionTimeouts.Inc()
}

func (registry *Registry) ObserveRetention(duration time.Duration, deleted int64, failed bool) {
	registry.retentionDuration.Observe(duration.Seconds())
	if deleted > 0 {
		registry.retentionDeleted.Add(float64(deleted))
	}
	if failed {
		registry.retentionErrors.Inc()
	}
}

func (registry *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(registry.prometheusRegistry, promhttp.HandlerOpts{})
}

func (registry *Registry) ObserveHTTPRequest(method, route string, statusCode int, duration time.Duration) {
	registry.httpRequests.WithLabelValues(method, route, strconv.Itoa(statusCode)).Inc()
	registry.httpDuration.WithLabelValues(method, route).Observe(duration.Seconds())
}

func (registry *Registry) SetDatabaseReady(ready bool) {
	if ready {
		registry.databaseReady.Set(1)
		return
	}
	registry.databaseReady.Set(0)
}
