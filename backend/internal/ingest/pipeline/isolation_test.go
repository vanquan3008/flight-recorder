package pipeline

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func TestInvalidEventIsIsolatedFromItsBatch(t *testing.T) {
	var mutex sync.Mutex
	stored := []string{}
	storeCalls := 0
	ingestionPipeline := New(Config{QueueSize: 16, QueueMaxBytes: 1 << 20, BatchSize: 8, Workers: 1, EnqueueTimeout: time.Second, BatchTimeout: time.Second, FlushInterval: 50 * time.Millisecond},
		func(_ context.Context, events []telemetry.Event) error {
			mutex.Lock()
			defer mutex.Unlock()
			storeCalls++
			for _, event := range events {
				if event.ID == "bad" {
					return fmt.Errorf("insert event batch: %w", telemetry.ErrInvalidEvent)
				}
			}
			for _, event := range events {
				stored = append(stored, event.ID)
			}
			return nil
		}, nil)
	ingestionPipeline.Start()
	defer func() { _ = ingestionPipeline.Shutdown(context.Background()) }()

	events := []telemetry.Event{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "bad"}, {ID: "d"}, {ID: "e"}, {ID: "f"}, {ID: "g"}}
	persisted, rejected, submitError := ingestionPipeline.Submit(context.Background(), events)
	if submitError != nil || persisted != 7 || rejected != 1 {
		t.Fatalf("persisted=%d rejected=%d error=%v, want 7, 1 and no retryable error", persisted, rejected, submitError)
	}

	mutex.Lock()
	defer mutex.Unlock()
	slices.Sort(stored)
	if want := []string{"a", "b", "c", "d", "e", "f", "g"}; !slices.Equal(stored, want) {
		t.Fatalf("stored %v, want %v", stored, want)
	}
	// Halving 8 -> 4 -> 2 -> 1 takes at most 7 calls; retrying invalid content
	// would multiply that by the retry count.
	if storeCalls > 7 {
		t.Fatalf("store called %d times; invalid content was retried", storeCalls)
	}
}

func TestEstimateEventBytesProducesProportionalEstimates(t *testing.T) {
	small := telemetry.Event{
		ID:          "small-id",
		ServiceName: "frontend",
		Operation:   "GET /home",
	}
	large := telemetry.Event{
		ID:           "large-id",
		ServiceName:  "payment-service",
		Operation:    "POST /checkout",
		ErrorMessage: string(make([]byte, 4096)),
		Attributes: map[string]any{
			"http.headers": "Bearer token...",
			"retry_count":  3,
		},
	}

	smallSize := estimateEventBytes(small)
	largeSize := estimateEventBytes(large)

	if smallSize < 256 {
		t.Fatalf("small size estimate %d should include envelope base", smallSize)
	}
	if largeSize <= smallSize+4096 {
		t.Fatalf("large size estimate %d should account for 4KB payload", largeSize)
	}
}

func TestEstimateEventBytesKeepsValidDeepAndUnknownValuesSmall(t *testing.T) {
	nested := map[string]any{"leaf": "value"}
	for depth := range 16 {
		if depth%2 == 0 {
			nested = map[string]any{"child": nested}
		} else {
			nested = map[string]any{"list": []any{nested}}
		}
	}
	event := telemetry.Event{
		ID:          "deep-id",
		ServiceName: "checkout",
		Attributes: map[string]any{
			"nested":  nested,
			"unknown": struct{ Value int }{Value: 1},
			"floats":  []float64{1, 2, 3},
		},
		LogBody: struct{}{},
	}

	if size := estimateEventBytes(event); size <= 0 || size > 4096 {
		t.Fatalf("estimate %d must stay within the queue budget for a valid event", size)
	}
}
