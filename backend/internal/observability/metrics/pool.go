package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// PoolSnapshot is a point-in-time reading of a database connection pool.
type PoolSnapshot struct {
	Acquired int32 // connections checked out right now
	Idle     int32
	Max      int32
	// EmptyAcquires counts checkouts that had to wait because no connection was
	// free; with AcquireWait it is the direct measure of pool saturation.
	EmptyAcquires int64
	AcquireWait   time.Duration
}

// WatchPool exposes a pool's saturation. The snapshot is read only when
// Prometheus scrapes, so it adds nothing to the request path.
func (registry *Registry) WatchPool(snapshot func() PoolSnapshot) {
	registry.prometheusRegistry.MustRegister(poolCollector{snapshot: snapshot})
}

type poolCollector struct{ snapshot func() PoolSnapshot }

var (
	poolConnections = prometheus.NewDesc("tracepilot_db_pool_connections", "Database pool connections by state.", []string{"state"}, nil)
	poolEmpty       = prometheus.NewDesc("tracepilot_db_pool_empty_acquires_total", "Connection checkouts that waited because the pool had none free.", nil, nil)
	poolWait        = prometheus.NewDesc("tracepilot_db_pool_acquire_wait_seconds_total", "Total time spent waiting for a free database connection.", nil, nil)
)

func (poolCollector) Describe(descriptions chan<- *prometheus.Desc) {
	descriptions <- poolConnections
	descriptions <- poolEmpty
	descriptions <- poolWait
}

func (collector poolCollector) Collect(samples chan<- prometheus.Metric) {
	reading := collector.snapshot()
	samples <- prometheus.MustNewConstMetric(poolConnections, prometheus.GaugeValue, float64(reading.Acquired), "acquired")
	samples <- prometheus.MustNewConstMetric(poolConnections, prometheus.GaugeValue, float64(reading.Idle), "idle")
	samples <- prometheus.MustNewConstMetric(poolConnections, prometheus.GaugeValue, float64(reading.Max), "max")
	samples <- prometheus.MustNewConstMetric(poolEmpty, prometheus.CounterValue, float64(reading.EmptyAcquires))
	samples <- prometheus.MustNewConstMetric(poolWait, prometheus.CounterValue, reading.AcquireWait.Seconds())
}
