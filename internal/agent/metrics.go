package agent

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	linesRead      prometheus.Counter
	exportedLines  prometheus.Counter
	exportFailures prometheus.Counter
	spoolFailures  prometheus.Counter
	handler        http.Handler
}

func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()
	metrics := &Metrics{
		linesRead:      prometheus.NewCounter(prometheus.CounterOpts{Namespace: "flight_agent", Name: "lines_read_total", Help: "Log lines read from the source file."}),
		exportedLines:  prometheus.NewCounter(prometheus.CounterOpts{Namespace: "flight_agent", Name: "lines_exported_total", Help: "Log lines acknowledged by Flight."}),
		exportFailures: prometheus.NewCounter(prometheus.CounterOpts{Namespace: "flight_agent", Name: "export_failures_total", Help: "OTLP export attempts that failed."}),
		spoolFailures:  prometheus.NewCounter(prometheus.CounterOpts{Namespace: "flight_agent", Name: "spool_failures_total", Help: "Disk spool encode, enqueue, or acknowledgement failures."}),
	}
	registry.MustRegister(metrics.linesRead, metrics.exportedLines, metrics.exportFailures, metrics.spoolFailures)
	metrics.handler = promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	return metrics
}

func (metrics *Metrics) ObserveRead(count int)   { metrics.linesRead.Add(float64(count)) }
func (metrics *Metrics) ObserveExport(count int) { metrics.exportedLines.Add(float64(count)) }
func (metrics *Metrics) ObserveExportFailure()   { metrics.exportFailures.Inc() }
func (metrics *Metrics) ObserveSpoolFailure()    { metrics.spoolFailures.Inc() }

func (metrics *Metrics) Handler() http.Handler { return metrics.handler }
