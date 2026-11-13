package notification

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
)

type deliveryStoreStub struct{}

func (deliveryStoreStub) RecordDeliveryAttempt(context.Context, string, string, string, int, string, bool) error {
	return nil
}

func (deliveryStoreStub) ClaimDueDeliveries(context.Context, int) ([]notificationdomain.DeliveryAttempt, error) {
	return nil, nil
}

type recordingStoreStub struct {
	called  chan struct{}
	success atomic.Bool
	status  atomic.Int32
}

type observerStub struct{ outcomes chan string }

func (observer observerStub) ObserveWebhookAttempt(outcome string) {
	observer.outcomes <- outcome
}

func (store *recordingStoreStub) RecordDeliveryAttempt(_ context.Context, _ string, _ string, _ string, status int, _ string, success bool) error {
	store.status.Store(int32(status))
	store.success.Store(success)
	close(store.called)
	return nil
}

func (store *recordingStoreStub) ClaimDueDeliveries(context.Context, int) ([]notificationdomain.DeliveryAttempt, error) {
	return nil, nil
}

type retryStoreStub struct {
	claimed   atomic.Bool
	attempts  atomic.Int32
	serverURL string
	secret    string
	processed chan struct{}
}

func (store *retryStoreStub) ClaimDueDeliveries(context.Context, int) ([]notificationdomain.DeliveryAttempt, error) {
	if store.claimed.Swap(true) {
		return nil, nil
	}
	return []notificationdomain.DeliveryAttempt{{Delivery: notificationdomain.Delivery{ID: "delivery-1", WebhookID: "webhook-1"}, ProjectID: "project-1", URL: store.serverURL, Secret: store.secret, Payload: []byte(`{"event":"incident.opened"}`)}}, nil
}

func (store *retryStoreStub) RecordDeliveryAttempt(_ context.Context, _ string, _ string, _ string, _ int, _ string, success bool) error {
	if success {
		store.attempts.Add(1)
		close(store.processed)
	}
	return nil
}

func TestDispatchSignsWithRawWebhookSecret(t *testing.T) {
	secret := "raw-webhook-secret-that-is-not-a-digest"
	payload := []byte(`{"event":"incident.opened"}`)
	handler := http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(body)
		want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if request.Header.Get("X-TracePilot-Signature") != want {
			t.Errorf("signature = %q, want %q", request.Header.Get("X-TracePilot-Signature"), want)
		}
		responseWriter.WriteHeader(http.StatusNoContent)
	})
	testServer := httptest.NewServer(handler)
	defer testServer.Close()

	observer := observerStub{outcomes: make(chan string, 1)}
	service := NewService(deliveryStoreStub{}, slog.Default(), Config{Observer: observer})
	service.client = testServer.Client()
	service.dispatch(context.Background(), "delivery-1", notificationdomain.WebhookRecord{
		ID: "webhook-1", URL: testServer.URL, Secret: secret,
	}, payload, "project-1")
	select {
	case outcome := <-observer.outcomes:
		if outcome != "success" {
			t.Fatalf("observer outcome = %q", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("success outcome was not observed")
	}
}

func TestDispatchRecordsNon2xxAsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		responseWriter.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	store := &recordingStoreStub{called: make(chan struct{})}
	service := NewService(store, slog.Default())
	service.client = server.Client()
	service.dispatch(context.Background(), "delivery-1", notificationdomain.WebhookRecord{
		ID: "webhook-1", URL: server.URL, Secret: "failure-secret-that-is-long",
	}, []byte(`{"event":"incident.opened"}`), "project-1")
	select {
	case <-store.called:
	case <-time.After(time.Second):
		t.Fatal("dispatch did not record failure")
	}
	if store.success.Load() {
		t.Fatal("non-2xx response was recorded as success")
	}
	if store.status.Load() != http.StatusServiceUnavailable {
		t.Fatalf("recorded status = %d, want %d", store.status.Load(), http.StatusServiceUnavailable)
	}
}

func TestRetryWorkerClaimsAndDispatches(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		responseWriter.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	store := &retryStoreStub{serverURL: server.URL, secret: "worker-secret-that-is-long", processed: make(chan struct{})}
	service := NewService(store, slog.Default(), Config{PollInterval: time.Hour, Workers: 1})
	service.client = server.Client()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workerDone := make(chan error, 1)
	go func() { workerDone <- service.Run(ctx) }()
	select {
	case <-time.After(time.Second):
		t.Fatal("retry worker did not process a claim")
	case <-store.processed:
	}
	if store.attempts.Load() != 1 {
		cancel()
		<-workerDone
		t.Fatalf("successful attempts = %d, want 1", store.attempts.Load())
	}
	cancel()
	if err := <-workerDone; err != nil {
		t.Fatal(err)
	}
}
