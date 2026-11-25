package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func (registry *Registry) ObserveWebhookAttempt(outcome string) {
	switch outcome {
	case "success", "failure", "claim_error":
	default:
		outcome = "other"
	}
	registry.webhookAttempts.WithLabelValues(outcome).Inc()
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

func (registry *Registry) SetIngestionInflight(count int64) {
	if count < 0 {
		count = 0
	}
	registry.ingestionInflight.Set(float64(count))
}

func (registry *Registry) ObserveIngestionShutdown(outcome string) {
	switch outcome {
	case "completed", "timeout":
	default:
		outcome = "other"
	}
	registry.ingestionShutdowns.WithLabelValues(outcome).Inc()
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

// ObserveRetentionGroup counts records one cleanup removed from a data group:
// telemetry, metrics, incidents, audit or deliveries.
func (registry *Registry) ObserveRetentionGroup(group string, deleted int64) {
	if deleted > 0 {
		registry.retentionByGroup.WithLabelValues(group).Add(float64(deleted))
	}
}

func (registry *Registry) ObserveIncidentDetection(duration time.Duration, count int, failed bool) {
	registry.detectionDuration.Observe(duration.Seconds())
	if failed {
		registry.detectionRuns.WithLabelValues("error").Inc()
		return
	}
	if count > 0 {
		registry.detectionRuns.WithLabelValues("detected").Inc()
		return
	}
	registry.detectionRuns.WithLabelValues("empty").Inc()
}

// ObserveSpanRollup records one refresh: the minutes it recomputed and
// whether it failed. Failures leave minutes dirty, so queries read them from
// raw spans; a steady error rate means slower, not wrong, performance pages.
func (registry *Registry) ObserveSpanRollup(minutes int, failed bool) {
	if minutes > 0 {
		registry.rollupMinutes.Add(float64(minutes))
	}
	outcome := "ok"
	if failed {
		outcome = "error"
	}
	registry.rollupRuns.WithLabelValues(outcome).Inc()
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

func (registry *Registry) ObserveAuthCache(kind, result string) {
	if registry == nil || registry.authCacheRequests == nil {
		return
	}
	registry.authCacheRequests.WithLabelValues(kind, result).Inc()
}

func (registry *Registry) ObserveGateWait(gate string, duration time.Duration) {
	if registry == nil || registry.gateWaitDuration == nil {
		return
	}
	registry.gateWaitDuration.WithLabelValues(gate).Observe(duration.Seconds())
}

func (registry *Registry) IncGateActive(gate string) {
	if registry == nil || registry.gateActiveSlots == nil {
		return
	}
	registry.gateActiveSlots.WithLabelValues(gate).Inc()
}

func (registry *Registry) DecGateActive(gate string) {
	if registry == nil || registry.gateActiveSlots == nil {
		return
	}
	registry.gateActiveSlots.WithLabelValues(gate).Dec()
}
