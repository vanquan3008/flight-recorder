package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

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

func (pipeline *Pipeline) worker(ctx context.Context) {
	defer pipeline.waitGroup.Done()
	for {
		first, open := <-pipeline.queue
		if !open {
			return
		}
		pipeline.setQueueDepth(len(pipeline.queue))
		batch := make([]item, 1, pipeline.batchSize)
		batch[0] = first
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
				pipeline.setQueueDepth(len(pipeline.queue))
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

// persist stores a batch and acknowledges every item in it. Batches mix events
// from many requests, so when storage rejects event content the batch is split
// until the invalid events are isolated; the rest still commit.
func (pipeline *Pipeline) persist(ctx context.Context, batch []item) {
	storeError := pipeline.storeWithRetry(ctx, batch)
	if errors.Is(storeError, telemetry.ErrInvalidEvent) && len(batch) > 1 {
		middle := len(batch) / 2
		pipeline.persist(ctx, batch[:middle])
		pipeline.persist(ctx, batch[middle:])
		return
	}
	for _, queued := range batch {
		pipeline.releaseBytes(queued.bytes)
		queued.ack <- storeError
	}
}

// storeWithRetry retries temporary failures only; invalid content fails the
// same way on every attempt.
func (pipeline *Pipeline) storeWithRetry(ctx context.Context, batch []item) error {
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
		if storeError == nil || errors.Is(storeError, telemetry.ErrInvalidEvent) || ctx.Err() != nil {
			break
		}
		pipeline.observeRetry()
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
	pipeline.observeBatch(time.Since(startedAt), storeError != nil)
	return storeError
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
	pipeline.setIngestionBytes(pipeline.inflightBytes.Add(-count))
	pipeline.setIngestionInflight(pipeline.inflightEvents.Add(-1))
}

func (pipeline *Pipeline) setQueueDepth(depth int) {
	if pipeline.metrics != nil {
		pipeline.metrics.SetIngestionQueueDepth(depth)
	}
}

func (pipeline *Pipeline) setIngestionBytes(bytes int64) {
	if pipeline.metrics != nil {
		pipeline.metrics.SetIngestionBytes(bytes)
	}
}

func (pipeline *Pipeline) setIngestionInflight(count int64) {
	if pipeline.metrics != nil {
		pipeline.metrics.SetIngestionInflight(count)
	}
}

func (pipeline *Pipeline) observeAdmissionTimeout() {
	if pipeline.metrics != nil {
		pipeline.metrics.ObserveAdmissionTimeout()
	}
}

func (pipeline *Pipeline) observeRetry() {
	if pipeline.metrics != nil {
		pipeline.metrics.ObserveRetry()
	}
}

func (pipeline *Pipeline) observeBatch(duration time.Duration, failed bool) {
	if pipeline.metrics != nil {
		pipeline.metrics.ObserveBatch(duration, failed)
	}
}

func (pipeline *Pipeline) observeShutdown(outcome string) {
	pipeline.shutdownMetricOnce.Do(func() {
		if pipeline.metrics != nil {
			pipeline.metrics.ObserveIngestionShutdown(outcome)
		}
	})
}

func estimateBytes(value any) int64 {
	if event, ok := value.(telemetry.Event); ok {
		return estimateEventBytes(event)
	}
	estimator := estimateEncoders.Get().(*estimateEncoder)
	estimator.buffer.Reset()
	encodeError := estimator.encoder.Encode(value)
	encodedBytes := estimator.buffer.Len()
	if estimator.buffer.Cap() <= maxPooledEstimateBytes {
		estimateEncoders.Put(estimator)
	}
	if encodeError != nil || encodedBytes == 0 {
		return 1
	}
	return int64(encodedBytes)
}
