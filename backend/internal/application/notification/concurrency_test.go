package notification

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
)

type busyDeliveryStore struct {
	deliveryStoreStub
	claims int
}

func (store *busyDeliveryStore) ClaimDueDeliveries(context.Context, int) ([]notificationdomain.DeliveryAttempt, error) {
	store.claims++
	return []notificationdomain.DeliveryAttempt{{URL: "https://example.com/hook"}}, nil
}

func TestWorkerLimitHoldsAcrossPollingIntervals(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const interval = 10 * time.Millisecond
		store := &busyDeliveryStore{}
		service := NewService(store, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{Workers: 1, PollInterval: interval})
		var calls atomic.Int32
		service.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			<-request.Context().Done()
			return nil, request.Context().Err()
		})}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- service.Run(ctx) }()
		synctest.Wait()
		time.Sleep(4 * interval)
		cancel()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if calls.Load() != 1 || store.claims != 1 {
			t.Fatalf("a busy worker started %d requests from %d claims; want one of each", calls.Load(), store.claims)
		}
	})
}
