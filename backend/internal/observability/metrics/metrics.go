package metrics

import (
	"runtime"

	"github.com/prometheus/client_golang/prometheus"
)

type Registry struct {
	prometheusRegistry     *prometheus.Registry
	httpRequests           *prometheus.CounterVec
	httpDuration           *prometheus.HistogramVec
	databaseReady          prometheus.Gauge
	ingestedRecords        *prometheus.CounterVec
	rejectedRecords        *prometheus.CounterVec
	ingestionQueue         prometheus.Gauge
	ingestionBytes         prometheus.Gauge
	ingestionInflight      prometheus.Gauge
	ingestionShutdowns     *prometheus.CounterVec
	batchDuration          prometheus.Histogram
	storageErrors          prometheus.Counter
	ingestionRetries       prometheus.Counter
	admissionTimeouts      prometheus.Counter
	otlpRequests           *prometheus.CounterVec
	otlpDuration           *prometheus.HistogramVec
	retentionDeleted       prometheus.Counter
	retentionBacklog       prometheus.Gauge
	retentionErrors        prometheus.Counter
	retentionDuration      prometheus.Histogram
	syslogRecords          *prometheus.CounterVec
	syslogConnections      prometheus.Gauge
	detectionRuns          *prometheus.CounterVec
	detectionDuration      prometheus.Histogram
	detectionLag           prometheus.Gauge
	detectionCheckpointAge prometheus.Gauge
	webhookAttempts        *prometheus.CounterVec
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
	ingestionInflight := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "tracepilot",
		Name:      "ingestion_inflight_events",
		Help:      "Events admitted to ingestion but not yet acknowledged by storage.",
	})
	ingestionShutdowns := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tracepilot",
		Name:      "ingestion_shutdowns_total",
		Help:      "Ingestion shutdown drain outcomes.",
	}, []string{"outcome"})
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
	syslogRecords := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tracepilot", Name: "security_syslog_records_total", Help: "Syslog records processed by outcome and recognized format.",
	}, []string{"outcome", "format"})
	syslogConnections := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "tracepilot", Name: "security_syslog_connections", Help: "Open Syslog TCP/TLS connections.",
	})
	detectionRuns := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tracepilot", Name: "security_detection_runs_total", Help: "Security detection worker runs by outcome.",
	}, []string{"outcome"})
	detectionDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "tracepilot", Name: "security_detection_duration_seconds", Help: "Security detection worker evaluation duration.", Buckets: prometheus.DefBuckets,
	})
	detectionLag := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "tracepilot", Name: "security_detection_lag_seconds", Help: "Age difference between the newest security event and the durable detection checkpoint.",
	})
	detectionCheckpointAge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "tracepilot", Name: "security_detection_checkpoint_age_seconds", Help: "Age of the last successful security detection checkpoint.",
	})
	webhookAttempts := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tracepilot", Name: "webhook_delivery_attempts_total", Help: "Outbound webhook delivery attempts by bounded outcome.",
	}, []string{"outcome"})
	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "tracepilot",
		Name:      "build_info",
		Help:      "TracePilot build and runtime information.",
	}, []string{"version", "commit", "build_time", "go_version"})

	prometheusRegistry.MustRegister(httpRequests, httpDuration, databaseReady, ingestedRecords, rejectedRecords, ingestionQueue, ingestionBytes, ingestionInflight, ingestionShutdowns, batchDuration, storageErrors, ingestionRetries, admissionTimeouts, otlpRequests, otlpDuration, retentionDeleted, retentionBacklog, retentionErrors, retentionDuration, syslogRecords, syslogConnections, detectionRuns, detectionDuration, detectionLag, detectionCheckpointAge, webhookAttempts, buildInfo)
	buildInfo.WithLabelValues(version, commit, buildTime, runtime.Version()).Set(1)

	return &Registry{
		prometheusRegistry:     prometheusRegistry,
		httpRequests:           httpRequests,
		httpDuration:           httpDuration,
		databaseReady:          databaseReady,
		ingestedRecords:        ingestedRecords,
		rejectedRecords:        rejectedRecords,
		ingestionQueue:         ingestionQueue,
		ingestionBytes:         ingestionBytes,
		ingestionInflight:      ingestionInflight,
		ingestionShutdowns:     ingestionShutdowns,
		batchDuration:          batchDuration,
		storageErrors:          storageErrors,
		ingestionRetries:       ingestionRetries,
		admissionTimeouts:      admissionTimeouts,
		otlpRequests:           otlpRequests,
		otlpDuration:           otlpDuration,
		retentionDeleted:       retentionDeleted,
		retentionBacklog:       retentionBacklog,
		retentionErrors:        retentionErrors,
		retentionDuration:      retentionDuration,
		syslogRecords:          syslogRecords,
		syslogConnections:      syslogConnections,
		detectionRuns:          detectionRuns,
		detectionDuration:      detectionDuration,
		detectionLag:           detectionLag,
		detectionCheckpointAge: detectionCheckpointAge,
		webhookAttempts:        webhookAttempts,
	}
}
