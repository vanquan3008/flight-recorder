package pipeline

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func TestMetricsPipelineSubmitAndShutdownAreSafeConcurrently(t *testing.T) {
	pipeline := NewMetricsPipeline(MetricsPipelineConfig{QueueSize: 1, BatchSize: 1, Workers: 1}, func(context.Context, []telemetry.MetricDataPoint) error {
		return nil
	})
	pipeline.Start()

	var submitters sync.WaitGroup
	point := telemetry.MetricDataPoint{MetricName: "requests"}
	for range 32 {
		submitters.Add(1)
		go func() {
			defer submitters.Done()
			for range 32 {
				_, _, _ = pipeline.SubmitMetrics(context.Background(), []telemetry.MetricDataPoint{point})
			}
		}()
	}

	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- pipeline.Shutdown(context.Background())
	}()
	submitters.Wait()
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("shutdown failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("metrics pipeline shutdown did not complete")
	}
}

func TestMetricsByteBudgetIncludesActiveWritesAndIsReleased(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{{"success", nil}, {"failure", errors.New("database unavailable")}} {
		t.Run(test.name, func(t *testing.T) {
			point := telemetry.MetricDataPoint{MetricName: "requests", Attributes: map[string]any{"label": strings.Repeat("x", 4096)}}
			budget := estimateMetricBytes(point)
			started, release := make(chan struct{}), make(chan struct{})
			pipe := NewMetricsPipeline(MetricsPipelineConfig{QueueSize: 4, BatchSize: 1, Workers: 1, QueueMaxBytes: budget}, func(ctx context.Context, _ []telemetry.MetricDataPoint) error {
				started <- struct{}{}
				select {
				case <-release:
					return test.err
				case <-ctx.Done():
					return ctx.Err()
				}
			})
			pipe.Start()
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if err := pipe.Shutdown(ctx); err != nil {
					t.Errorf("shutdown: %v", err)
				}
			})
			first := make(chan error, 1)
			go func() {
				_, _, err := pipe.SubmitMetrics(context.Background(), []telemetry.MetricDataPoint{point})
				first <- err
			}()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("store did not start")
			}
			if persisted, _, err := pipe.SubmitMetrics(context.Background(), []telemetry.MetricDataPoint{point}); persisted != 0 || err == nil {
				t.Fatalf("active write did not hold byte budget: persisted=%d error=%v", persisted, err)
			}
			close(release)
			if err := <-first; (err == nil) != (test.err == nil) {
				t.Fatalf("first request error = %v, want %v", err, test.err)
			}
			if pipe.queued.Load() != 0 || !pipe.byteBudget.TryAcquire(budget) {
				t.Fatal("completed write leaked its point or byte budget")
			}
			pipe.byteBudget.Release(budget)
			if pipe.DroppedTotal() != 1 {
				t.Fatalf("dropped = %d, want 1", pipe.DroppedTotal())
			}
		})
	}
}

func TestMetricsSubmitAcknowledgesOnlyAfterStoreCommits(t *testing.T) {
	storeStarted, releaseStore := make(chan struct{}), make(chan struct{})
	pipe := NewMetricsPipeline(MetricsPipelineConfig{QueueSize: 4, BatchSize: 1, Workers: 1}, func(context.Context, []telemetry.MetricDataPoint) error {
		close(storeStarted)
		<-releaseStore
		return nil
	})
	pipe.Start()
	result := make(chan [3]any, 1)
	go func() {
		persisted, rejected, err := pipe.SubmitMetrics(context.Background(), []telemetry.MetricDataPoint{{MetricName: "requests"}})
		result <- [3]any{persisted, rejected, err}
	}()
	<-storeStarted
	select {
	case <-result:
		t.Fatal("SubmitMetrics returned before the store committed")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseStore)
	if got := <-result; got[0] != 1 || got[1] != 0 || got[2] != nil {
		t.Fatalf("result = %v, want 1 persisted", got)
	}
	shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pipe.Shutdown(shutdown); err != nil {
		t.Fatal(err)
	}
}

func TestMetricsStorageFailureIsRetryableNotAcknowledged(t *testing.T) {
	pipe := NewMetricsPipeline(MetricsPipelineConfig{QueueSize: 4, BatchSize: 1, Workers: 1}, func(context.Context, []telemetry.MetricDataPoint) error {
		return errors.New("database unavailable")
	})
	pipe.Start()
	persisted, rejected, err := pipe.SubmitMetrics(context.Background(), []telemetry.MetricDataPoint{{MetricName: "requests"}})
	if persisted != 0 || rejected != 0 || err == nil {
		t.Fatalf("storage outage: persisted=%d rejected=%d error=%v; the client must be told to retry", persisted, rejected, err)
	}
	shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = pipe.Shutdown(shutdown)
}

func TestInvalidMetricRequestDoesNotFailRequestsMergedWithIt(t *testing.T) {
	var writes [][]string
	pipe := NewMetricsPipeline(MetricsPipelineConfig{QueueSize: 4, BatchSize: 8}, func(_ context.Context, points []telemetry.MetricDataPoint) error {
		names := make([]string, len(points))
		for index, point := range points {
			names[index] = point.MetricName
			if point.MetricName == "bad" {
				return telemetry.ErrInvalidEvent
			}
		}
		writes = append(writes, names)
		return nil
	})
	names := []string{"first", "bad", "last"}
	batches := make([]metricBatch, len(names))
	for index, name := range names {
		batches[index] = metricBatch{points: []telemetry.MetricDataPoint{{MetricName: name}}, ack: make(chan error, 1)}
	}
	pipe.queued.Store(int64(len(names)))
	pipe.flush(context.Background(), batches)
	for index, wantInvalid := range []bool{false, true, false} {
		if got := errors.Is(<-batches[index].ack, telemetry.ErrInvalidEvent); got != wantInvalid {
			t.Fatalf("request %q invalid = %t, want %t", names[index], got, wantInvalid)
		}
	}
	if len(writes) != 2 || pipe.queued.Load() != 0 {
		t.Fatalf("writes = %v, queued = %d; valid requests must still commit", writes, pipe.queued.Load())
	}
}

func TestMetricsShutdownTimeoutFailsInFlightRequestsRetryably(t *testing.T) {
	storeStarted := make(chan struct{})
	pipe := NewMetricsPipeline(MetricsPipelineConfig{QueueSize: 4, BatchSize: 1, Workers: 1}, func(ctx context.Context, _ []telemetry.MetricDataPoint) error {
		close(storeStarted)
		<-ctx.Done()
		return ctx.Err()
	})
	pipe.Start()
	submitError := make(chan error, 1)
	go func() {
		_, _, err := pipe.SubmitMetrics(context.Background(), []telemetry.MetricDataPoint{{MetricName: "requests"}})
		submitError <- err
	}()
	<-storeStarted
	shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := pipe.Shutdown(shutdown); err == nil {
		t.Fatal("shutdown reported success while a write was still running")
	}
	select {
	case err := <-submitError:
		if err == nil {
			t.Fatal("an unfinished write was acknowledged")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight request was never answered after shutdown")
	}
}

func TestMetricByteEstimateIncludesHistogramAndMetadata(t *testing.T) {
	point := telemetry.MetricDataPoint{Attributes: map[string]any{"label": strings.Repeat("x", 4096)}}
	withoutBuckets := estimateMetricBytes(point)
	point.HistBuckets = []telemetry.HistogramBucket{{UpperBound: math.Inf(1), Count: 1}}
	if size := estimateMetricBytes(point); size <= 4096 || size != withoutBuckets+16 {
		t.Fatalf("histogram estimate = %d, metadata estimate = %d", size, withoutBuckets)
	}
}

func TestMetricsPipelineReadyOnlyWhileAcceptingRequests(t *testing.T) {
	pipeline := NewMetricsPipeline(MetricsPipelineConfig{QueueSize: 1, BatchSize: 1, Workers: 1}, func(context.Context, []telemetry.MetricDataPoint) error { return nil })
	if pipeline.Ready() {
		t.Fatal("ready before Start")
	}
	pipeline.Start()
	if !pipeline.Ready() {
		t.Fatal("not ready after Start")
	}
	if err := pipeline.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if pipeline.Ready() {
		t.Fatal("still ready after Shutdown")
	}
}

func TestMetricsRequestThatCanNeverFitIsRejectedNotRetried(t *testing.T) {
	stored := false
	pipeline := NewMetricsPipeline(MetricsPipelineConfig{QueueSize: 4, BatchSize: 4, Workers: 1, QueueMaxBytes: 1024}, func(context.Context, []telemetry.MetricDataPoint) error {
		stored = true
		return nil
	})
	pipeline.Start()
	defer func() { _ = pipeline.Shutdown(context.Background()) }()

	persisted, rejected, err := pipeline.SubmitMetrics(context.Background(), []telemetry.MetricDataPoint{{MetricName: "requests"}})
	if err != nil || persisted != 0 || rejected != 1 || stored {
		t.Fatalf("persisted=%d rejected=%d err=%v stored=%t; a request over the byte budget cannot succeed on retry", persisted, rejected, err, stored)
	}
	if pipeline.DroppedTotal() != 0 {
		t.Fatalf("a rejected request must not count as dropped by backpressure: %d", pipeline.DroppedTotal())
	}
}
