package pipeline

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"golang.org/x/sync/semaphore"
)

var errOversizedBatch = errors.New("metrics request exceeds the pipeline byte budget")

const batchStoreTimeout = 5 * time.Second

type MetricsStoreFunc func(context.Context, []telemetry.MetricDataPoint) error

type MetricsPipeline struct {
	queue         chan metricBatch
	byteBudget    *semaphore.Weighted
	maxBytes      int64
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

type MetricsPipelineConfig struct {
	QueueSize     int
	QueueMaxBytes int64
	BatchSize     int
	Workers       int
	Metrics       *metrics.Registry
}

type metricBatch struct {
	points []telemetry.MetricDataPoint
	bytes  int64
	ack    chan error
}

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
	if cfg.QueueMaxBytes <= 0 {
		cfg.QueueMaxBytes = 64 << 20
	}
	return &MetricsPipeline{
		queue:      make(chan metricBatch, cfg.QueueSize),
		byteBudget: semaphore.NewWeighted(cfg.QueueMaxBytes),
		maxBytes:   cfg.QueueMaxBytes,
		store:      store,
		batchSize:  cfg.BatchSize,
		workers:    cfg.Workers,
		metrics:    cfg.Metrics,
		maxQueued:  int64(cfg.QueueSize) * int64(cfg.BatchSize),
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
		for i := 0; i < p.workers; i++ {
			p.waitGroup.Add(1)
			go p.worker(workerCtx)
		}
	})
}

func (p *MetricsPipeline) SubmitMetrics(ctx context.Context, points []telemetry.MetricDataPoint) (persisted, rejected int, err error) {
	if len(points) == 0 {
		return 0, 0, nil
	}
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	batch, admitError := p.admit(points)
	if errors.Is(admitError, errOversizedBatch) {
		return 0, len(points), nil
	}
	if admitError != nil {
		return 0, 0, admitError
	}
	select {
	case writeError := <-batch.ack:
		switch {
		case writeError == nil:
			return len(points), 0, nil
		case errors.Is(writeError, telemetry.ErrInvalidEvent):
			return 0, len(points), nil
		default:
			return 0, 0, writeError
		}
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	}
}

func (p *MetricsPipeline) admit(points []telemetry.MetricDataPoint) (metricBatch, error) {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	if !p.ready.Load() {
		return metricBatch{}, ErrUnavailable
	}
	count := int64(len(points))
	if !p.reservePoints(count) {
		return metricBatch{}, p.reject(len(points), "queue_full", errors.New("metrics pipeline queue is full"))
	}
	var bytes int64
	for _, point := range points {
		bytes += estimateMetricBytes(point)
	}
	if bytes > p.maxBytes {
		p.queued.Add(-count)
		return metricBatch{}, errOversizedBatch
	}
	if !p.byteBudget.TryAcquire(bytes) {
		p.queued.Add(-count)
		return metricBatch{}, p.reject(len(points), "queue_bytes", errors.New("metrics pipeline byte budget is full"))
	}
	batch := metricBatch{points: points, bytes: bytes, ack: make(chan error, 1)}
	select {
	case p.queue <- batch:
		return batch, nil
	default:
		p.byteBudget.Release(bytes)
		p.queued.Add(-count)
		return metricBatch{}, p.reject(len(points), "queue_full", errors.New("metrics pipeline queue is full"))
	}
}

func (p *MetricsPipeline) reservePoints(count int64) bool {
	for {
		queued := p.queued.Load()
		if queued+count > p.maxQueued {
			return false
		}
		if p.queued.CompareAndSwap(queued, queued+count) {
			return true
		}
	}
}

func (p *MetricsPipeline) reject(count int, reason string, cause error) error {
	p.dropped.Add(int64(count))
	if p.metrics != nil {
		p.metrics.ObserveRejected("metrics", reason, count)
	}
	return cause
}

func (p *MetricsPipeline) Ready() bool { return p.ready.Load() }

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
	for {
		select {
		case <-ctx.Done():
			return
		case first, open := <-p.queue:
			if !open {
				return
			}
			batches := []metricBatch{first}
			points := len(first.points)

		collect:
			for points < p.batchSize {
				select {
				case <-ctx.Done():
					p.flush(ctx, batches)
					return
				case more, open := <-p.queue:
					if !open {
						break collect
					}
					batches = append(batches, more)
					points += len(more.points)
				default:
					break collect
				}
			}
			p.flush(ctx, batches)
		}
	}
}

func (p *MetricsPipeline) flush(ctx context.Context, batches []metricBatch) {
	totalPoints := 0
	for _, batch := range batches {
		totalPoints += len(batch.points)
	}

	points := make([]telemetry.MetricDataPoint, 0, totalPoints)
	for _, batch := range batches {
		points = append(points, batch.points...)
	}

	storeCtx, cancel := context.WithTimeout(ctx, batchStoreTimeout)
	defer cancel()

	writeError := p.store(storeCtx, points)

	if errors.Is(writeError, telemetry.ErrInvalidEvent) && len(batches) > 1 {
		for _, batch := range batches {
			p.flush(ctx, []metricBatch{batch})
		}
		return
	}

	if writeError != nil && !errors.Is(writeError, telemetry.ErrInvalidEvent) && p.metrics != nil {
		p.metrics.ObserveRejected("metrics", "storage", len(points))
	}

	for _, batch := range batches {
		p.queued.Add(-int64(len(batch.points)))
		p.byteBudget.Release(batch.bytes)
		select {
		case batch.ack <- writeError:
		default:
		}
	}
}
