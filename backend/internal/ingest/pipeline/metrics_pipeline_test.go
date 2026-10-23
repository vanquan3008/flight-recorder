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
				_, _ = pipeline.SubmitMetrics(context.Background(), []telemetry.MetricDataPoint{point})
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
				select {
				case started <- struct{}{}:
				case <-ctx.Done():
					return ctx.Err()
				}
				select {
				case <-release:
					return test.err
				case <-ctx.Done():
					return ctx.Err()
				}
			})
			pipe.Start()
			t.Cleanup(func() {
				close(release)
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if err := pipe.Shutdown(ctx); err != nil {
					t.Errorf("shutdown: %v", err)
				}
			})
			if accepted, err := pipe.SubmitMetrics(context.Background(), []telemetry.MetricDataPoint{point}); accepted != 1 || err != nil {
				t.Fatalf("first batch: accepted=%d error=%v", accepted, err)
			}
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("store did not start")
			}
			if accepted, err := pipe.SubmitMetrics(context.Background(), []telemetry.MetricDataPoint{point}); accepted != 0 || err == nil {
				t.Fatalf("active write did not hold byte budget: accepted=%d error=%v", accepted, err)
			}
			release <- struct{}{}
			deadline := time.Now().Add(time.Second)
			for pipe.queued.Load() != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
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

func TestMetricByteEstimateIncludesHistogramAndMetadata(t *testing.T) {
	point := telemetry.MetricDataPoint{Attributes: map[string]any{"label": strings.Repeat("x", 4096)}}
	withoutBuckets := estimateMetricBytes(point)
	point.HistBuckets = []telemetry.HistogramBucket{{UpperBound: math.Inf(1), Count: 1}}
	if size := estimateMetricBytes(point); size <= 4096 || size != withoutBuckets+16 {
		t.Fatalf("histogram estimate = %d, metadata estimate = %d", size, withoutBuckets)
	}
}
