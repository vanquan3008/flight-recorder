package pipeline

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
)

// batchStoreTimeout is the maximum time allowed for a single metrics batch write.
const batchStoreTimeout = 5 * time.Second

// MetricsStoreFunc persists a batch of metric data points.
type MetricsStoreFunc func(context.Context, []telemetry.MetricDataPoint) error

// MetricsPipeline is a lightweight, non-durable ingest path for OTLP metrics.
// ponytail: metrics may be dropped on backpressure/storage failure; add a
// durable queue only when a metrics durability SLO and volume benchmark exist.
// Unlike the event pipeline it does NOT wait for persistence acknowledgement —
// metrics are best-effort: a full queue simply drops incoming data points and
// increments the rejected counter. This matches how most observability backends
// treat high-cardinality metrics under backpressure.
type MetricsPipeline struct {
	queue         chan []telemetry.MetricDataPoint
	store         MetricsStoreFunc
	batchSize     int
	workers       int
	metrics       *metrics.Registry
	maxQueued     int64
	waitGroup     sync.WaitGroup
	cancelWorkers context.CancelFunc
	startOnce     sync.Once
	shutdownOnce  sync.Once
	stateMu       sync.RWMutex
	closed        bool
	ready         atomic.Bool
	dropped       atomic.Int64
	queued        atomic.Int64
}

// MetricsPipelineConfig holds tuning parameters for MetricsPipeline.
type MetricsPipelineConfig struct {
	QueueSize int
	BatchSize int
	Workers   int
	Metrics   *metrics.Registry
}

// NewMetricsPipeline creates a MetricsPipeline. Call Start before SubmitMetrics.
func NewMetricsPipeline(cfg MetricsPipelineConfig, store MetricsStoreFunc) *MetricsPipeline {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1024
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 512
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 2
	}
	return &MetricsPipeline{
		queue:     make(chan []telemetry.MetricDataPoint, cfg.QueueSize),
		store:     store,
		batchSize: cfg.BatchSize,
		workers:   cfg.Workers,
		metrics:   cfg.Metrics,
		maxQueued: int64(cfg.QueueSize) * int64(cfg.BatchSize),
	}
}

func (p *MetricsPipeline) Start() {
	p.startOnce.Do(func() {
		workerCtx, cancel := context.WithCancel(context.Background())
		p.stateMu.Lock()
		defer p.stateMu.Unlock()
		if p.closed {
			cancel()
			return
		}
		p.cancelWorkers = cancel
		p.ready.Store(true)
		for range p.workers {
			p.waitGroup.Add(1)
			go p.worker(workerCtx)
		}
	})
}

// SubmitMetrics enqueues data points for asynchronous persistence. If the queue
// is full it drops the batch and returns an error immediately rather than
// blocking — callers should treat this as a temporary backpressure signal.
func (p *MetricsPipeline) SubmitMetrics(ctx context.Context, points []telemetry.MetricDataPoint) (int, error) {
	if len(points) == 0 {
		return 0, nil
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	// Keep the read lock across the non-blocking send. Shutdown takes the
	// write lock before closing the channel, so a sender can never race with
	// close and panic with "send on closed channel".
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	if !p.ready.Load() {
		return 0, ErrUnavailable
	}
	pointCount := int64(len(points))
	for {
		queued := p.queued.Load()
		if queued+pointCount > p.maxQueued || !p.queued.CompareAndSwap(queued, queued+pointCount) {
			if queued+pointCount > p.maxQueued {
				p.dropped.Add(pointCount)
				if p.metrics != nil {
					p.metrics.ObserveRejected("metrics", "queue_full", len(points))
				}
				return 0, errors.New("metrics pipeline queue is full")
			}
			continue
		}
		break
	}
	select {
	case p.queue <- points:
		return len(points), nil
	default:
		p.queued.Add(-pointCount)
		p.dropped.Add(pointCount)
		if p.metrics != nil {
			p.metrics.ObserveRejected("metrics", "queue_full", len(points))
		}
		return 0, errors.New("metrics pipeline queue is full")
	}
}

// DroppedTotal returns the cumulative count of data points that were dropped
// due to a full queue.
func (p *MetricsPipeline) DroppedTotal() int64 { return p.dropped.Load() }

func (p *MetricsPipeline) Shutdown(ctx context.Context) error {
	p.shutdownOnce.Do(func() {
		p.stateMu.Lock()
		p.closed = true
		p.ready.Store(false)
		close(p.queue)
		p.stateMu.Unlock()
	})
	done := make(chan struct{})
	go func() {
		p.waitGroup.Wait()
		close(done)
	}()
	select {
	case <-done:
		if p.cancelWorkers != nil {
			p.cancelWorkers()
		}
		return nil
	case <-ctx.Done():
		if p.cancelWorkers != nil {
			p.cancelWorkers()
		}
		return ctx.Err()
	}
}

func (p *MetricsPipeline) worker(ctx context.Context) {
	defer p.waitGroup.Done()
	batch := make([]telemetry.MetricDataPoint, 0, p.batchSize)
	for {
		first, open := <-p.queue
		if !open {
			if len(batch) > 0 {
				p.flush(ctx, batch)
			}
			return
		}
		batch = append(batch, first...)
		for len(batch) < p.batchSize {
			select {
			case more, ok := <-p.queue:
				if !ok {
					p.flush(ctx, batch)
					return
				}
				batch = append(batch, more...)
			default:
				goto flush
			}
		}
	flush:
		p.flush(ctx, batch)
		batch = batch[:0]
	}
}

func (p *MetricsPipeline) flush(ctx context.Context, points []telemetry.MetricDataPoint) {
	if len(points) == 0 {
		return
	}
	defer p.queued.Add(-int64(len(points)))
	storeCtx, cancel := context.WithTimeout(ctx, batchStoreTimeout)
	defer cancel()
	// Best-effort: ignore errors on the metrics path.
	if err := p.store(storeCtx, points); err != nil && p.metrics != nil {
		p.metrics.ObserveRejected("metrics", "storage", len(points))
	}
}
