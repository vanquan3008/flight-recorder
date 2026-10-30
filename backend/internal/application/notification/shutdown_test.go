package notification

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"

	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

// contextRecordingStore remembers whether the context it was given was already cancelled.
type contextRecordingStore struct {
	deliveryStoreStub
	mu        sync.Mutex
	cancelled bool
	success   bool
	calls     int
}

func (store *contextRecordingStore) RecordDeliveryAttempt(ctx context.Context, _, _, _ string, _ int, _ string, success bool) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.calls++
	store.cancelled = ctx.Err() != nil
	store.success = success
	return ctx.Err()
}

// A delivery that finishes while shutdown cancels the service context must still
// be recorded; otherwise its lease expires and the notification is sent again.
func TestDispatchRecordsTheOutcomeEvenAfterShutdownCancelsTheContext(t *testing.T) {
	for _, test := range []struct {
		name        string
		transport   func(cancel context.CancelFunc) roundTripFunc
		wantSuccess bool
	}{
		{"delivered", func(cancel context.CancelFunc) roundTripFunc {
			return func(request *http.Request) (*http.Response, error) {
				cancel() // shutdown begins while the request is in flight
				return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Request: request}, nil
			}
		}, true},
		{"failed", func(cancel context.CancelFunc) roundTripFunc {
			return func(*http.Request) (*http.Response, error) {
				cancel()
				return nil, errors.New("connection reset")
			}
		}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			store := &contextRecordingStore{}
			service := NewService(store, slog.Default())
			service.client = &http.Client{Transport: test.transport(cancel)}

			service.dispatch(ctx, "delivery-1", notificationdomain.WebhookRecord{ID: "webhook-1", URL: "https://hooks.example/x", Secret: "secret"}, []byte("{}"), "project-1")

			store.mu.Lock()
			defer store.mu.Unlock()
			if store.calls != 1 {
				t.Fatalf("outcome was recorded %d times, want 1", store.calls)
			}
			if store.cancelled {
				t.Fatal("the outcome was written with the cancelled shutdown context, so it would be lost")
			}
			if store.success != test.wantSuccess {
				t.Fatalf("recorded success = %t, want %t", store.success, test.wantSuccess)
			}
		})
	}
}
