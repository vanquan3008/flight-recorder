package pipeline

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
)

func TestOverloadedRequestIsRejectedWithoutQueueingAnyOfItsEvents(t *testing.T) {
	var mutex sync.Mutex
	var stored []string
	storeStarted := make(chan struct{}, 4)
	releaseStore := make(chan struct{})
	ingestionPipeline := New(Config{QueueSize: 2, QueueMaxBytes: 1 << 20, BatchSize: 1, Workers: 1, EnqueueTimeout: 20 * time.Millisecond, BatchTimeout: time.Second, FlushInterval: time.Millisecond}, func(_ context.Context, events []telemetry.Event) error {
		storeStarted <- struct{}{}
		<-releaseStore
		mutex.Lock()
		defer mutex.Unlock()
		for _, event := range events {
			stored = append(stored, event.ID)
		}
		return nil
	}, metrics.New("test", "test", "test"))
	ingestionPipeline.Start()

	first := make(chan error, 1)
	go func() {
		_, _, err := ingestionPipeline.Submit(context.Background(), []telemetry.Event{{ID: "held"}})
		first <- err
	}()
	<-storeStarted

	overloaded, cancelOverloaded := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelOverloaded()
	persisted, _, err := ingestionPipeline.Submit(overloaded, []telemetry.Event{{ID: "over-1"}, {ID: "over-2"}, {ID: "over-3"}})
	if persisted != 0 || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("overloaded request = (%d, %v), want (0, ErrUnavailable)", persisted, err)
	}
	if depth := len(ingestionPipeline.queue); depth != 0 {
		t.Fatalf("queue depth = %d; a rejected request must not leave events behind", depth)
	}

	close(releaseStore)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
	defer cancelShutdown()
	if err := ingestionPipeline.Shutdown(shutdownContext); err != nil {
		t.Fatal(err)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if !slices.Equal(stored, []string{"held"}) {
		t.Fatalf("stored %v; events of the rejected request were persisted", stored)
	}
}

func TestRequestLargerThanTheQueueIsAdmittedInParts(t *testing.T) {
	var mutex sync.Mutex
	stored := 0
	ingestionPipeline := New(Config{QueueSize: 2, QueueMaxBytes: 1 << 20, BatchSize: 1, Workers: 1, EnqueueTimeout: time.Second, BatchTimeout: time.Second, FlushInterval: time.Millisecond}, func(_ context.Context, events []telemetry.Event) error {
		mutex.Lock()
		stored += len(events)
		mutex.Unlock()
		return nil
	}, metrics.New("test", "test", "test"))
	ingestionPipeline.Start()
	defer func() {
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
		defer cancelShutdown()
		_ = ingestionPipeline.Shutdown(shutdownContext)
	}()

	events := make([]telemetry.Event, 7)
	for index := range events {
		events[index] = telemetry.Event{ID: string(rune('a' + index))}
	}
	persisted, rejected, err := ingestionPipeline.Submit(context.Background(), events)
	if err != nil || persisted != len(events) || rejected != 0 {
		t.Fatalf("large request = (%d, %d, %v), want all %d persisted", persisted, rejected, err, len(events))
	}
	mutex.Lock()
	defer mutex.Unlock()
	if stored != len(events) {
		t.Fatalf("stored %d events, want %d", stored, len(events))
	}
}
