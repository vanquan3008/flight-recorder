package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"golang.org/x/sync/semaphore"
)

var ErrUnavailable = errors.New("ingestion pipeline unavailable")

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
	queue              chan item
	byteBudget         *semaphore.Weighted
	maxBytes           int64
	inflightBytes      atomic.Int64
	inflightEvents     atomic.Int64
	enqueueTimeout     time.Duration
	batchSize          int
	batchTimeout       time.Duration
	flushInterval      time.Duration
	workers            int
	store              StoreFunc
	metrics            *metrics.Registry
	submitMutex        sync.RWMutex
	accepting          bool
	waitGroup          sync.WaitGroup
	cancelWorkers      context.CancelFunc
	startOnce          sync.Once
	shutdownOnce       sync.Once
	shutdownMetricOnce sync.Once
	closed             bool
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

	// One buffered result channel per request avoids allocating one channel for
	// every event while still allowing workers to finish batches independently.
	ack := make(chan error, len(events))
	accepted := 0
	pipeline.submitMutex.RLock()
	if !pipeline.accepting {
		pipeline.submitMutex.RUnlock()
		return 0, ErrUnavailable
	}
	for _, event := range events {
		eventBytes := estimateBytes(event)
		if eventBytes > pipeline.maxBytes {
			pipeline.submitMutex.RUnlock()
			return pipeline.waitForAcks(ctx, ack, accepted, ErrUnavailable)
		}
		if acquireError := pipeline.byteBudget.Acquire(admissionContext, eventBytes); acquireError != nil {
			if errors.Is(acquireError, context.DeadlineExceeded) {
				pipeline.observeAdmissionTimeout()
			}
			pipeline.submitMutex.RUnlock()
			return pipeline.waitForAcks(ctx, ack, accepted, errors.Join(ErrUnavailable, acquireError))
		}
		pipeline.setIngestionBytes(pipeline.inflightBytes.Add(eventBytes))
		pipeline.setIngestionInflight(pipeline.inflightEvents.Add(1))
		queued := item{event: event, ack: ack, bytes: eventBytes}
		select {
		case pipeline.queue <- queued:
			accepted++
			pipeline.setQueueDepth(len(pipeline.queue))
		case <-admissionContext.Done():
			if errors.Is(admissionContext.Err(), context.DeadlineExceeded) {
				pipeline.observeAdmissionTimeout()
			}
			pipeline.releaseBytes(queued.bytes)
			pipeline.submitMutex.RUnlock()
			return pipeline.waitForAcks(ctx, ack, accepted, errors.Join(ErrUnavailable, admissionContext.Err()))
		}
	}
	pipeline.submitMutex.RUnlock()
	return pipeline.waitForAcks(ctx, ack, accepted, nil)
}

func (pipeline *Pipeline) waitForAcks(ctx context.Context, ack <-chan error, accepted int, admissionError error) (int, error) {
	persisted := 0
	resultError := admissionError
	for range accepted {
		select {
		case persistError := <-ack:
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
		pipeline.setQueueDepth(0)
		pipeline.setIngestionBytes(0)
		pipeline.setIngestionInflight(0)
		pipeline.observeShutdown("completed")
		return nil
	case <-ctx.Done():
		if pipeline.cancelWorkers != nil {
			pipeline.cancelWorkers()
		}
		pipeline.observeShutdown("timeout")
		return fmt.Errorf("drain %d event(s): %w", pipeline.inflightEvents.Load(), ctx.Err())
	}
}
