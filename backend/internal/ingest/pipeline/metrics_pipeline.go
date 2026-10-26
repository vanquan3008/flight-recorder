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

// batchStoreTimeout is the maximum time allowed for a single metrics batch write.
const batchStoreTimeout = 5 * time.Second

// MetricsStoreFunc persists a batch of metric data points.
type MetricsStoreFunc func(context.Context, []telemetry.MetricDataPoint) error

// MetricsPipeline carries customer OTLP metrics to PostgreSQL. Like the event
// pipeline it acknowledges only after the write commits: SubmitMetrics returns
// success once the points are stored, an error for a retryable failure, and a
// rejected count for points storage cannot hold. Admission is non-blocking, so
// a full queue fails fast and the exporter retries. TracePilot's own
// Prometheus metrics are a separate, in-process registry and are not stored here.
type MetricsPipeline struct {
	queue         chan metricBatch
	byteBudget    *semaphore.Weighted
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
	QueueSize     int
	QueueMaxBytes int64
	BatchSize     int
	Workers       int
	Metrics       *metrics.Registry
}

// metricBatch is one request's points. ack receives the write outcome exactly
// once and is buffered, so a worker never blocks on a submitter that gave up.
type metricBatch struct {
	points []telemetry.MetricDataPoint
	bytes  int64
	ack    chan error
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
	if cfg.QueueMaxBytes <= 0 {
		cfg.QueueMaxBytes = 64 << 20
	}
	return &MetricsPipeline{
		queue:      make(chan metricBatch, cfg.QueueSize),
		byteBudget: semaphore.NewWeighted(cfg.QueueMaxBytes),
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
		for range p.workers {
			p.waitGroup.Add(1)
			go p.worker(workerCtx)
		}
	})
}

// SubmitMetrics queues one request's points and waits for them to commit. It
// returns the committed count, the count storage rejected as invalid (retrying
// cannot help), and an error for temporary failures that must be retried.
// Writes are idempotent, so a retry after an unknown outcome cannot duplicate
// points.
func (p *MetricsPipeline) SubmitMetrics(ctx context.Context, points []telemetry.MetricDataPoint) (persisted, rejected int, err error) {
	if len(points) == 0 {
		return 0, 0, nil
	}
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	batch, admitError := p.admit(points)
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

// admit reserves queue room and byte budget without blocking, then enqueues.
// The read lock spans the send: Shutdown takes the write lock before closing
// the channel, so a sender can never race with close.
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

// reservePoints claims room for count points in the queue, or reports it full.
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

// DroppedTotal returns the cumulative count of data points refused at
// admission because the queue or byte budget was full.
func (p *MetricsPipeline) DroppedTotal() int64 { return p.dropped.Load() }

// Shutdown stops admission and lets the workers drain queued batches, so every
// accepted request still gets its outcome. When ctx expires first the workers
// are cancelled and the remaining requests fail with a retryable error.
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

// worker merges queued requests into batches of up to batchSize points. The
// loop ends when Shutdown closes the queue and every queued batch is flushed.
func (p *MetricsPipeline) worker(ctx context.Context) {
	defer p.waitGroup.Done()
	for first := range p.queue {
		batches, points := []metricBatch{first}, len(first.points)
	collect:
		for points < p.batchSize {
			select {
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

// flush writes the merged points and answers every request in the batch. When
// storage rejects the content of a merged write, each request is written alone
// so one bad request cannot fail the others.
func (p *MetricsPipeline) flush(ctx context.Context, batches []metricBatch) {
	var points []telemetry.MetricDataPoint
	for _, batch := range batches {
		points = append(points, batch.points...)
	}
	storeCtx, cancel := context.WithTimeout(ctx, batchStoreTimeout)
	writeError := p.store(storeCtx, points)
	cancel()
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
		batch.ack <- writeError
	}
}
