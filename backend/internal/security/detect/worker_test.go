package detect

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunEvaluatesImmediatelyAndStops(t *testing.T) {
	runtimeContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- Run(runtimeContext, slog.New(slog.NewTextHandler(io.Discard, nil)), func(context.Context) (int, error) {
			if calls.Add(1) == 1 {
				close(started)
			}
			return 0, nil
		}, time.Hour)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not evaluate immediately")
	}
	cancel()
	select {
	case runError := <-done:
		if runError != nil {
			t.Fatal(runError)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}
