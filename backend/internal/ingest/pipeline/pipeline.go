package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"golang.org/x/sync/semaphore"
)

var ErrUnavailable = errors.New("ingestion pipeline unavailable")

const maxPooledEstimateBytes = 64 << 10

type estimateEncoder struct {
	buffer  bytes.Buffer
	encoder *json.Encoder
}

var estimateEncoders = sync.Pool{New: func() any {
	estimator := &estimateEncoder{}
	estimator.encoder = json.NewEncoder(&estimator.buffer)
	return estimator
}}

type StoreFunc func(context.Context, []telemetry.Event) error

// Config contains the bounded-queue and batching settings for a Pipeline.
type Config struct {
	QueueSize      int
	QueueMaxBytes  int64
	BatchSize      int
	Workers        int
	EnqueueTimeout time.Duration
	BatchTimeout   time.Duration
	FlushInterval  time.Duration
}

type item struct {
	event telemetry.Event
	ack   chan error
	bytes int64
}

type Pipeline struct {
	queue          chan item
	byteBudget     *semaphore.Weighted
	maxBytes       int64
	inflightBytes  atomic.Int64
	inflightEvents atomic.Int64
	enqueueTimeout time.Duration
	batchSize      int
	batchTimeout   time.Duration
	flushInterval  time.Duration
	workers        int
	store          StoreFunc
	metrics        *metrics.Registry
	submitMutex    sync.RWMutex
	accepting      bool
	waitGroup      sync.WaitGroup
	cancelWorkers  context.CancelFunc
	startOnce      sync.Once
	shutdownOnce   sync.Once
	closed         bool
}

func New(configuration Config, store StoreFunc, metricRegistry *metrics.Registry) *Pipeline {
	return &Pipeline{
		queue:          make(chan item, configuration.QueueSize),
		byteBudget:     semaphore.NewWeighted(configuration.QueueMaxBytes),
		maxBytes:       configuration.QueueMaxBytes,
		enqueueTimeout: configuration.EnqueueTimeout,
		batchSize:      configuration.BatchSize,
		batchTimeout:   configuration.BatchTimeout,
		flushInterval:  configuration.FlushInterval,
		workers:        configuration.Workers,
		store:          store,
		metrics:        metricRegistry,
	}
}

func (pipeline *Pipeline) Start() {
	pipeline.startOnce.Do(func() {
		workerContext, cancelWorkers := context.WithCancel(context.Background())
		pipeline.submitMutex.Lock()
		if pipeline.closed {
			pipeline.submitMutex.Unlock()
			cancelWorkers()
			return
		}
		pipeline.cancelWorkers = cancelWorkers
		pipeline.accepting = true
		for range pipeline.workers {
			pipeline.waitGroup.Add(1)
			go pipeline.worker(workerContext)
		}
		pipeline.submitMutex.Unlock()
	})
}

func (pipeline *Pipeline) Ready() bool {
	pipeline.submitMutex.RLock()
	defer pipeline.submitMutex.RUnlock()
	return pipeline.accepting
}

// Submit waits until accepted events have committed. This is the Phase 2
// durability boundary: returning success never means "only queued in memory".
func (pipeline *Pipeline) Submit(ctx context.Context, events []telemetry.Event) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}
	admissionContext, cancelAdmission := context.WithTimeout(ctx, pipeline.enqueueTimeout)
	defer cancelAdmission()

	accepted := make([]item, 0, len(events))
	pipeline.submitMutex.RLock()
	if !pipeline.accepting {
		pipeline.submitMutex.RUnlock()
		return 0, ErrUnavailable
	}
	for _, event := range events {
		eventBytes := estimateBytes(event)
		if eventBytes > pipeline.maxBytes {
			pipeline.submitMutex.RUnlock()
			return pipeline.waitForAcks(ctx, accepted, ErrUnavailable)
		}
		if acquireError := pipeline.byteBudget.Acquire(admissionContext, eventBytes); acquireError != nil {
			if errors.Is(acquireError, context.DeadlineExceeded) {
				pipeline.metrics.ObserveAdmissionTimeout()
			}
			pipeline.submitMutex.RUnlock()
			return pipeline.waitForAcks(ctx, accepted, errors.Join(ErrUnavailable, acquireError))
		}
		pipeline.metrics.SetIngestionBytes(pipeline.inflightBytes.Add(eventBytes))
		pipeline.inflightEvents.Add(1)
		queued := item{event: event, ack: make(chan error, 1), bytes: eventBytes}
		select {
		case pipeline.queue <- queued:
			accepted = append(accepted, queued)
			pipeline.metrics.SetIngestionQueueDepth(len(pipeline.queue))
		case <-admissionContext.Done():
			if errors.Is(admissionContext.Err(), context.DeadlineExceeded) {
				pipeline.metrics.ObserveAdmissionTimeout()
			}
			pipeline.releaseBytes(queued.bytes)
			pipeline.submitMutex.RUnlock()
			return pipeline.waitForAcks(ctx, accepted, errors.Join(ErrUnavailable, admissionContext.Err()))
		}
	}
	pipeline.submitMutex.RUnlock()
	return pipeline.waitForAcks(ctx, accepted, nil)
}

func (pipeline *Pipeline) waitForAcks(ctx context.Context, accepted []item, admissionError error) (int, error) {
	persisted := 0
	resultError := admissionError
	for _, queued := range accepted {
		select {
		case persistError := <-queued.ack:
			if persistError == nil {
				persisted++
			} else {
				resultError = errors.Join(resultError, persistError)
			}
		case <-ctx.Done():
			return persisted, errors.Join(resultError, ctx.Err())
		}
	}
	return persisted, resultError
}

func (pipeline *Pipeline) Shutdown(ctx context.Context) error {
	pipeline.shutdownOnce.Do(func() {
		pipeline.submitMutex.Lock()
		pipeline.accepting = false
		pipeline.closed = true
		close(pipeline.queue)
		pipeline.submitMutex.Unlock()
	})

	done := make(chan struct{})
	go func() {
		pipeline.waitGroup.Wait()
		close(done)
	}()
	select {
	case <-done:
		if pipeline.cancelWorkers != nil {
			pipeline.cancelWorkers()
		}
		pipeline.metrics.SetIngestionQueueDepth(0)
		pipeline.metrics.SetIngestionBytes(0)
		return nil
	case <-ctx.Done():
		if pipeline.cancelWorkers != nil {
			pipeline.cancelWorkers()
		}
		return fmt.Errorf("drain %d event(s): %w", pipeline.inflightEvents.Load(), ctx.Err())
	}
}

func (pipeline *Pipeline) worker(ctx context.Context) {
	defer pipeline.waitGroup.Done()
	for {
		first, open := <-pipeline.queue
		if !open {
			return
		}
		pipeline.metrics.SetIngestionQueueDepth(len(pipeline.queue))
		batch := []item{first}
		timer := time.NewTimer(pipeline.flushInterval)
		queueOpen := true
	collect:
		for len(batch) < pipeline.batchSize {
			select {
			case queued, open := <-pipeline.queue:
				if !open {
					queueOpen = false
					break collect
				}
				batch = append(batch, queued)
				pipeline.metrics.SetIngestionQueueDepth(len(pipeline.queue))
			case <-timer.C:
				break collect
			case <-ctx.Done():
				break collect
			}
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		pipeline.persist(ctx, batch)
		if !queueOpen || ctx.Err() != nil {
			pipeline.rejectRemaining(ctx.Err())
			return
		}
	}
}

func (pipeline *Pipeline) persist(ctx context.Context, batch []item) {
	events := make([]telemetry.Event, len(batch))
	for index := range batch {
		events[index] = batch[index].event
	}

	startedAt := time.Now()
	var storeError error
	for attempt := 0; attempt < 3; attempt++ {
		batchContext, cancelBatch := context.WithTimeout(ctx, pipeline.batchTimeout)
		storeError = pipeline.store(batchContext, events)
		cancelBatch()
		if storeError == nil || ctx.Err() != nil {
			break
		}
		pipeline.metrics.ObserveRetry()
		retryDelay := time.NewTimer(retryBackoff(attempt))
		select {
		case <-retryDelay.C:
		case <-ctx.Done():
			if !retryDelay.Stop() {
				select {
				case <-retryDelay.C:
				default:
				}
			}
		}
	}
	pipeline.metrics.ObserveBatch(time.Since(startedAt), storeError != nil)
	for _, queued := range batch {
		pipeline.releaseBytes(queued.bytes)
		queued.ack <- storeError
	}
}

func retryBackoff(attempt int) time.Duration {
	base := 50 * time.Millisecond
	for range attempt {
		if base >= time.Second {
			base = time.Second
			break
		}
		base *= 2
	}
	// Keep retries bounded while spreading workers that fail against the same
	// dependency. The lower half of the interval is always respected.
	jitter := time.Duration(rand.Int64N(int64(base/2) + 1))
	return base/2 + jitter
}

func (pipeline *Pipeline) rejectRemaining(reason error) {
	if reason == nil {
		reason = ErrUnavailable
	}
	for queued := range pipeline.queue {
		pipeline.releaseBytes(queued.bytes)
		queued.ack <- reason
	}
}

func (pipeline *Pipeline) releaseBytes(count int64) {
	pipeline.byteBudget.Release(count)
	pipeline.metrics.SetIngestionBytes(pipeline.inflightBytes.Add(-count))
	pipeline.inflightEvents.Add(-1)
}

func estimateBytes(event telemetry.Event) int64 {
	estimator := estimateEncoders.Get().(*estimateEncoder)
	estimator.buffer.Reset()
	encodeError := estimator.encoder.Encode(event)
	encodedBytes := estimator.buffer.Len()
	if estimator.buffer.Cap() <= maxPooledEstimateBytes {
		estimateEncoders.Put(estimator)
	}
	if encodeError != nil || encodedBytes == 0 {
		return 1
	}
	return int64(encodedBytes)
}
