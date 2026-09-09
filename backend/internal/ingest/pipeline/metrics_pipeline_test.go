package pipeline

import (
	"context"
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
