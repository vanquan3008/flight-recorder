package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
)

func TestSubmitAcknowledgesOnlyAfterStoreCompletes(t *testing.T) {
	storeStarted := make(chan struct{})
	releaseStore := make(chan struct{})
	ingestionPipeline := New(Config{QueueSize: 4, QueueMaxBytes: 1 << 20, BatchSize: 1, Workers: 1, EnqueueTimeout: time.Second, BatchTimeout: time.Second, FlushInterval: time.Millisecond}, func(context.Context, []telemetry.Event) error {
		close(storeStarted)
		<-releaseStore
		return nil
	}, metrics.New("test", "test", "test"))
	ingestionPipeline.Start()
	if !ingestionPipeline.Ready() {
		t.Fatal("pipeline did not become ready")
	}

	result := make(chan struct {
		persisted int
		err       error
	}, 1)
	go func() {
		persisted, _, submitError := ingestionPipeline.Submit(context.Background(), []telemetry.Event{{ID: "event"}})
		result <- struct {
			persisted int
			err       error
		}{persisted: persisted, err: submitError}
	}()
	<-storeStarted
	select {
	case <-result:
		t.Fatal("Submit returned before the store committed")
	default:
	}
	close(releaseStore)
	submitResult := <-result
	if submitResult.err != nil || submitResult.persisted != 1 {
		t.Fatalf("submit result = %+v", submitResult)
	}

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
	defer cancelShutdown()
	if shutdownError := ingestionPipeline.Shutdown(shutdownContext); shutdownError != nil {
		t.Fatal(shutdownError)
	}
	if ingestionPipeline.Ready() {
		t.Fatal("pipeline remained ready after shutdown")
	}
}

func TestSubmitRejectsWhenQueueStaysFull(t *testing.T) {
	storeStarted := make(chan struct{}, 1)
	releaseStore := make(chan struct{})
	ingestionPipeline := New(Config{QueueSize: 1, QueueMaxBytes: 1 << 20, BatchSize: 1, Workers: 1, EnqueueTimeout: 20 * time.Millisecond, BatchTimeout: time.Second, FlushInterval: time.Millisecond}, func(context.Context, []telemetry.Event) error {
		storeStarted <- struct{}{}
		<-releaseStore
		return nil
	}, metrics.New("test", "test", "test"))
	ingestionPipeline.Start()

	firstResult := make(chan error, 1)
	go func() {
		_, _, submitError := ingestionPipeline.Submit(context.Background(), []telemetry.Event{{ID: "first"}})
		firstResult <- submitError
	}()
	<-storeStarted
	secondResult := make(chan error, 1)
	go func() {
		_, _, submitError := ingestionPipeline.Submit(context.Background(), []telemetry.Event{{ID: "second"}})
		secondResult <- submitError
	}()
	queueDeadline := time.Now().Add(time.Second)
	for len(ingestionPipeline.queue) != 1 && time.Now().Before(queueDeadline) {
		time.Sleep(time.Millisecond)
	}
	if len(ingestionPipeline.queue) != 1 {
		t.Fatal("second event did not fill the queue")
	}
	if persisted, _, submitError := ingestionPipeline.Submit(context.Background(), []telemetry.Event{{ID: "third"}}); persisted != 0 || !errors.Is(submitError, ErrUnavailable) {
		t.Fatalf("full queue result = (%d, %v)", persisted, submitError)
	}
	close(releaseStore)
	if submitError := <-firstResult; submitError != nil {
		t.Fatal(submitError)
	}
	<-storeStarted
	if submitError := <-secondResult; submitError != nil {
		t.Fatal(submitError)
	}

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
	defer cancelShutdown()
	if shutdownError := ingestionPipeline.Shutdown(shutdownContext); shutdownError != nil {
		t.Fatal(shutdownError)
	}
}

func TestStartAfterShutdownDoesNotReopenPipeline(t *testing.T) {
	ingestionPipeline := New(Config{QueueSize: 1, QueueMaxBytes: 1 << 20, BatchSize: 1, Workers: 1, EnqueueTimeout: time.Second, BatchTimeout: time.Second, FlushInterval: time.Millisecond}, func(context.Context, []telemetry.Event) error {
		return nil
	}, metrics.New("test", "test", "test"))

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
	defer cancelShutdown()
	if shutdownError := ingestionPipeline.Shutdown(shutdownContext); shutdownError != nil {
		t.Fatal(shutdownError)
	}
	ingestionPipeline.Start()
	if ingestionPipeline.Ready() {
		t.Fatal("pipeline reopened after shutdown")
	}
	if persisted, _, submitError := ingestionPipeline.Submit(context.Background(), []telemetry.Event{{ID: "event"}}); persisted != 0 || !errors.Is(submitError, ErrUnavailable) {
		t.Fatalf("submit after shutdown = (%d, %v)", persisted, submitError)
	}
}

func TestSubmitRecoversAfterTemporaryStoreOutage(t *testing.T) {
	attempts := 0
	ingestionPipeline := New(Config{QueueSize: 1, QueueMaxBytes: 1 << 20, BatchSize: 1, Workers: 1, EnqueueTimeout: time.Second, BatchTimeout: time.Second, FlushInterval: time.Millisecond}, func(context.Context, []telemetry.Event) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary store failure")
		}
		return nil
	}, metrics.New("test", "test", "test"))
	ingestionPipeline.Start()

	persisted, _, submitError := ingestionPipeline.Submit(context.Background(), []telemetry.Event{{ID: "event"}})
	if submitError != nil || persisted != 1 {
		t.Fatalf("submit result = (%d, %v)", persisted, submitError)
	}
	if attempts != 3 {
		t.Fatalf("store attempts = %d, want 3", attempts)
	}

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
	defer cancelShutdown()
	if shutdownError := ingestionPipeline.Shutdown(shutdownContext); shutdownError != nil {
		t.Fatal(shutdownError)
	}
}

func TestShutdownDrainsQueuedWork(t *testing.T) {
	storeStarted := make(chan struct{}, 1)
	releaseStore := make(chan struct{})
	storeCalls := 0
	ingestionPipeline := New(Config{QueueSize: 2, QueueMaxBytes: 1 << 20, BatchSize: 1, Workers: 1, EnqueueTimeout: time.Second, BatchTimeout: time.Second, FlushInterval: time.Millisecond}, func(context.Context, []telemetry.Event) error {
		storeCalls++
		if storeCalls == 1 {
			storeStarted <- struct{}{}
			<-releaseStore
		}
		return nil
	}, metrics.New("test", "test", "test"))
	ingestionPipeline.Start()

	first := make(chan error, 1)
	second := make(chan error, 1)
	go func() {
		_, _, err := ingestionPipeline.Submit(context.Background(), []telemetry.Event{{ID: "first"}})
		first <- err
	}()
	<-storeStarted
	go func() {
		_, _, err := ingestionPipeline.Submit(context.Background(), []telemetry.Event{{ID: "second"}})
		second <- err
	}()
	deadline := time.Now().Add(time.Second)
	for len(ingestionPipeline.queue) != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(ingestionPipeline.queue) != 1 {
		t.Fatal("second event was not queued")
	}

	shutdown := make(chan error, 1)
	go func() { shutdown <- ingestionPipeline.Shutdown(context.Background()) }()
	select {
	case err := <-shutdown:
		t.Fatalf("shutdown returned before queued work drained: %v", err)
	default:
	}
	close(releaseStore)
	for _, result := range []<-chan error{first, second, shutdown} {
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	}
	if storeCalls != 2 {
		t.Fatalf("store calls = %d, want 2", storeCalls)
	}
}

func BenchmarkSubmitBatch(b *testing.B) {
	events := make([]telemetry.Event, 256)
	for index := range events {
		events[index] = telemetry.Event{ID: "event", ServiceName: "benchmark", Attributes: map[string]any{"index": index}}
	}
	ingestionPipeline := New(Config{QueueSize: 4096, QueueMaxBytes: 64 << 20, BatchSize: 256, Workers: 4, EnqueueTimeout: time.Second, BatchTimeout: time.Second, FlushInterval: time.Millisecond}, func(context.Context, []telemetry.Event) error {
		return nil
	}, metrics.New("benchmark", "benchmark", "benchmark"))
	ingestionPipeline.Start()
	b.Cleanup(func() {
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
		defer cancelShutdown()
		if shutdownError := ingestionPipeline.Shutdown(shutdownContext); shutdownError != nil {
			b.Error(shutdownError)
		}
	})
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(parallel *testing.PB) {
		for parallel.Next() {
			if persisted, _, submitError := ingestionPipeline.Submit(context.Background(), events); submitError != nil || persisted != len(events) {
				b.Errorf("submit = (%d, %v)", persisted, submitError)
				return
			}
		}
	})
}

func TestSubmitRejectsAnEventThatCanNeverFitInsteadOfFailingTheRequest(t *testing.T) {
	var stored []string
	ingestionPipeline := New(Config{QueueSize: 4, QueueMaxBytes: 2048, BatchSize: 4, Workers: 1, EnqueueTimeout: time.Second, BatchTimeout: time.Second, FlushInterval: time.Millisecond}, func(_ context.Context, events []telemetry.Event) error {
		for _, event := range events {
			stored = append(stored, event.ID)
		}
		return nil
	}, metrics.New("test", "test", "test"))
	ingestionPipeline.Start()
	defer func() {
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
		defer cancelShutdown()
		_ = ingestionPipeline.Shutdown(shutdownContext)
	}()

	huge := telemetry.Event{ID: "huge", ErrorMessage: string(make([]byte, 8192))}
	persisted, rejected, submitError := ingestionPipeline.Submit(context.Background(), []telemetry.Event{{ID: "before"}, huge, {ID: "after"}})
	if submitError != nil {
		t.Fatalf("an oversized event failed the whole request with a retryable error: %v", submitError)
	}
	if persisted != 2 || rejected != 1 {
		t.Fatalf("persisted=%d rejected=%d, want 2 and 1", persisted, rejected)
	}
	if len(stored) != 2 {
		t.Fatalf("stored %v; the other events in the request must still commit", stored)
	}
}
