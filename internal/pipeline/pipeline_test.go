package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	flightmetrics "github.com/flight-observability/flight/internal/metrics"
	"github.com/flight-observability/flight/internal/telemetry"
)

func TestSubmitAcknowledgesOnlyAfterStoreCompletes(t *testing.T) {
	storeStarted := make(chan struct{})
	releaseStore := make(chan struct{})
	ingestionPipeline := New(4, 1<<20, 1, 1, time.Second, time.Second, time.Millisecond, func(context.Context, []telemetry.Event) error {
		close(storeStarted)
		<-releaseStore
		return nil
	}, flightmetrics.New("test", "test", "test"))
	ingestionPipeline.Start()
	if !ingestionPipeline.Ready() {
		t.Fatal("pipeline did not become ready")
	}

	result := make(chan struct {
		persisted int
		err       error
	}, 1)
	go func() {
		persisted, submitError := ingestionPipeline.Submit(context.Background(), []telemetry.Event{{ID: "event"}})
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
	ingestionPipeline := New(1, 1<<20, 1, 1, 20*time.Millisecond, time.Second, time.Millisecond, func(context.Context, []telemetry.Event) error {
		storeStarted <- struct{}{}
		<-releaseStore
		return nil
	}, flightmetrics.New("test", "test", "test"))
	ingestionPipeline.Start()

	firstResult := make(chan error, 1)
	go func() {
		_, submitError := ingestionPipeline.Submit(context.Background(), []telemetry.Event{{ID: "first"}})
		firstResult <- submitError
	}()
	<-storeStarted
	secondResult := make(chan error, 1)
	go func() {
		_, submitError := ingestionPipeline.Submit(context.Background(), []telemetry.Event{{ID: "second"}})
		secondResult <- submitError
	}()
	queueDeadline := time.Now().Add(time.Second)
	for len(ingestionPipeline.queue) != 1 && time.Now().Before(queueDeadline) {
		time.Sleep(time.Millisecond)
	}
	if len(ingestionPipeline.queue) != 1 {
		t.Fatal("second event did not fill the queue")
	}
	if persisted, submitError := ingestionPipeline.Submit(context.Background(), []telemetry.Event{{ID: "third"}}); persisted != 0 || !errors.Is(submitError, ErrUnavailable) {
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

func BenchmarkSubmitBatch(b *testing.B) {
	events := make([]telemetry.Event, 256)
	for index := range events {
		events[index] = telemetry.Event{ID: "event", ServiceName: "benchmark", Attributes: map[string]any{"index": index}}
	}
	ingestionPipeline := New(4096, 64<<20, 256, 4, time.Second, time.Second, time.Millisecond, func(context.Context, []telemetry.Event) error {
		return nil
	}, flightmetrics.New("benchmark", "benchmark", "benchmark"))
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
			if persisted, submitError := ingestionPipeline.Submit(context.Background(), events); submitError != nil || persisted != len(events) {
				b.Errorf("submit = (%d, %v)", persisted, submitError)
				return
			}
		}
	})
}
