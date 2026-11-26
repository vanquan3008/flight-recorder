package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
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
	slotBudget         *semaphore.Weighted
	slotCapacity       int64
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
	onPersist          func([]telemetry.Event)
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

// New sizes the queue for QueueSize waiting events plus the batches workers hold
// while persisting. A queue slot stays reserved until its event is acknowledged,
// so admitted events never block on the channel.
func New(configuration Config, store StoreFunc, metricRegistry *metrics.Registry) *Pipeline {
	slotCapacity := int64(max(configuration.QueueSize, 1) + max(configuration.Workers, 0)*max(configuration.BatchSize, 0))
	return &Pipeline{
		queue:          make(chan item, slotCapacity),
		slotBudget:     semaphore.NewWeighted(slotCapacity),
		slotCapacity:   slotCapacity,
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
// It reports committed events, events storage rejected as invalid (retrying
// them cannot help), and an error for temporary failures that must be retried.
//
// A request is admitted as a whole: queue slots and bytes for all of its events
// are reserved before any event is queued, so an overloaded pipeline rejects it
// without persisting work the client will send again. Only a request larger
// than the whole queue is admitted in parts.
func (pipeline *Pipeline) Submit(ctx context.Context, events []telemetry.Event) (persisted, rejected int, err error) {
	if len(events) == 0 {
		return 0, 0, nil
	}
	admissionContext, cancelAdmission := context.WithTimeout(ctx, pipeline.enqueueTimeout)
	defer cancelAdmission()

	ack := make(chan error, len(events))
	accepted := 0
	// Events that can never fit the byte budget are rejected like events storage
	// refuses: retrying the request cannot help, so it must not fail with 503.
	oversized := 0
	wait := func(admissionError error) (int, int, error) {
		persisted, rejected, err := pipeline.waitForAcks(ctx, ack, accepted, admissionError)
		return persisted, rejected + oversized, err
	}
	pipeline.submitMutex.RLock()
	if !pipeline.accepting {
		pipeline.submitMutex.RUnlock()
		return 0, 0, ErrUnavailable
	}
	part := make([]item, 0, min(len(events), int(pipeline.slotCapacity)))
	var partBytes int64
	admit := func() error {
		if len(part) == 0 {
			return nil
		}
		if reserveError := pipeline.reserve(admissionContext, int64(len(part)), partBytes); reserveError != nil {
			return reserveError
		}
		for _, queued := range part {
			pipeline.queue <- queued
		}
		accepted += len(part)
		pipeline.setQueueDepth(len(pipeline.queue))
		part, partBytes = part[:0], 0
		return nil
	}
	for _, event := range events {
		eventBytes := estimateBytes(event)
		if eventBytes > pipeline.maxBytes {
			oversized++
			continue
		}
		if int64(len(part)) == pipeline.slotCapacity || partBytes+eventBytes > pipeline.maxBytes {
			if admitError := admit(); admitError != nil {
				pipeline.submitMutex.RUnlock()
				return wait(errors.Join(ErrUnavailable, admitError))
			}
		}
		part = append(part, item{event: event, ack: ack, bytes: eventBytes})
		partBytes += eventBytes
	}
	admitError := admit()
	pipeline.submitMutex.RUnlock()
	if admitError != nil {
		return wait(errors.Join(ErrUnavailable, admitError))
	}
	return wait(nil)
}

// reserve takes queue slots and bytes for a whole part of a request, or
// neither. Slots are always taken before bytes, so two requests cannot each
// hold what the other is waiting for.
func (pipeline *Pipeline) reserve(ctx context.Context, events, bytes int64) error {
	if acquireError := pipeline.slotBudget.Acquire(ctx, events); acquireError != nil {
		pipeline.observeReserveFailure(acquireError)
		return acquireError
	}
	if acquireError := pipeline.byteBudget.Acquire(ctx, bytes); acquireError != nil {
		pipeline.slotBudget.Release(events)
		pipeline.observeReserveFailure(acquireError)
		return acquireError
	}
	pipeline.setIngestionBytes(pipeline.inflightBytes.Add(bytes))
	pipeline.setIngestionInflight(pipeline.inflightEvents.Add(events))
	return nil
}

func (pipeline *Pipeline) observeReserveFailure(reason error) {
	if errors.Is(reason, context.DeadlineExceeded) {
		pipeline.observeAdmissionTimeout()
	}
}

func (pipeline *Pipeline) waitForAcks(ctx context.Context, ack <-chan error, accepted int, admissionError error) (persisted, rejected int, err error) {
	err = admissionError
	for range accepted {
		select {
		case persistError := <-ack:
			switch {
			case persistError == nil:
				persisted++
			case errors.Is(persistError, telemetry.ErrInvalidEvent):
				rejected++
			default:
				err = errors.Join(err, persistError)
			}
		case <-ctx.Done():
			return persisted, rejected, errors.Join(err, ctx.Err())
		}
	}
	return persisted, rejected, err
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

// SetOnPersist registers an optional listener that receives batches of events after
// storage commits them successfully.
func (pipeline *Pipeline) SetOnPersist(listener func([]telemetry.Event)) {
	pipeline.onPersist = listener
}
