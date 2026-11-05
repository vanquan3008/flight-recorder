package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func TestStreamHubSubscribeAndBroadcast(t *testing.T) {
	hub := NewStreamHub()
	projectID := "proj-1"
	envID := "env-1"

	events, unsubscribe, admitted := hub.Subscribe(projectID, envID)
	if !admitted {
		t.Fatal("stream subscription was rejected")
	}
	defer unsubscribe()

	testEvent := StreamEvent{
		Type:          "log",
		ProjectID:     projectID,
		EnvironmentID: envID,
		Payload:       map[string]string{"message": "hello streaming"},
	}

	hub.Broadcast(testEvent)

	select {
	case received := <-events:
		if received.Type != "log" {
			t.Fatalf("expected log, got %s", received.Type)
		}
		if received.ProjectID != projectID {
			t.Fatalf("expected project %s, got %s", projectID, received.ProjectID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for stream event")
	}
}

func TestStreamHubBroadcastEvents(t *testing.T) {
	hub := NewStreamHub()
	projectID := "proj-2"
	envID := "env-2"

	events, unsubscribe, admitted := hub.Subscribe(projectID, envID)
	if !admitted {
		t.Fatal("stream subscription was rejected")
	}
	defer unsubscribe()

	hub.BroadcastEvents([]telemetry.Event{
		{
			ID:            "ev-1",
			ProjectID:     projectID,
			Environment:   "staging",
			EnvironmentID: envID,
			ServiceName:   "checkout",
			Signal:        telemetry.SignalLog,
			Timestamp:     time.Now(),
			Severity:      "INFO",
			LogBody:       "user clicked buy",
		},
		{
			ID:            "ev-2",
			ProjectID:     projectID,
			Environment:   "staging",
			EnvironmentID: envID,
			ServiceName:   "payment",
			Signal:        telemetry.SignalTrace,
			Timestamp:     time.Now(),
			Status:        "ERROR",
			ErrorType:     "PaymentTimeout",
		},
	})

	receivedCount := 0
	for receivedCount < 2 {
		select {
		case ev := <-events:
			receivedCount++
			if ev.ProjectID != projectID {
				t.Fatalf("expected project %s, got %s", projectID, ev.ProjectID)
			}
			if ev.EnvironmentID != envID {
				t.Fatalf("expected environment ID %s, got %s", envID, ev.EnvironmentID)
			}
			if ev.Type == "log" {
				payload, ok := ev.Payload.(map[string]any)
				if !ok || payload["body"] != "user clicked buy" {
					t.Fatalf("log body = %#v", ev.Payload)
				}
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("expected 2 events, got %d", receivedCount)
		}
	}
}

func TestStreamHubDoesNotCrossEnvironment(t *testing.T) {
	hub := NewStreamHub()
	events, unsubscribe, admitted := hub.Subscribe("project", "production-id")
	if !admitted {
		t.Fatal("stream subscription was rejected")
	}
	defer unsubscribe()

	hub.Broadcast(StreamEvent{ProjectID: "project", EnvironmentID: "staging-id"})
	hub.Broadcast(StreamEvent{ProjectID: "project"})
	select {
	case event := <-events:
		t.Fatalf("received event from another or unknown environment: %+v", event)
	default:
	}
}

func TestStreamHubBoundsSubscribers(t *testing.T) {
	hub := NewStreamHub()
	unsubscribe := make([]func(), 0, maxStreamSubscribers)
	for range maxStreamSubscribers {
		_, stop, admitted := hub.Subscribe("project", "environment")
		if !admitted {
			t.Fatal("subscription was rejected before reaching the limit")
		}
		unsubscribe = append(unsubscribe, stop)
	}
	if _, _, admitted := hub.Subscribe("project", "environment"); admitted {
		t.Fatal("subscription exceeded the configured limit")
	}
	for _, stop := range unsubscribe {
		stop()
	}
}

func TestStreamHubCloseRejectsNewSubscribers(t *testing.T) {
	hub := NewStreamHub()
	hub.Close()
	if _, _, admitted := hub.Subscribe("project", "environment"); admitted {
		t.Fatal("closed stream hub admitted a new subscriber")
	}
	select {
	case <-hub.done:
	default:
		t.Fatal("closing the stream hub did not signal subscribers")
	}
}

func TestStreamHandlerStopsWhenHubCloses(t *testing.T) {
	hub := NewStreamHub()
	server := &Server{streamHub: hub}
	ctx := context.Background()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/live/stream", nil).WithContext(ctx)
	recorder := &deadlineResponseRecorder{ResponseRecorder: httptest.NewRecorder()}
	done := make(chan struct{})
	go func() {
		server.streamLiveTelemetry(recorder, request, auth.Principal{ProjectID: "project", EnvironmentID: "environment"})
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	hub.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream handler did not stop when the hub closed")
	}
}

type deadlineResponseRecorder struct {
	*httptest.ResponseRecorder
}

func (*deadlineResponseRecorder) SetWriteDeadline(time.Time) error { return nil }

func TestStreamLiveTelemetryHandler(t *testing.T) {
	hub := NewStreamHub()
	server := &Server{
		streamHub: hub,
	}

	projectID := "proj-test"
	envID := "env-test"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/live/stream", nil).WithContext(ctx)
	recorder := &deadlineResponseRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		server.streamLiveTelemetry(recorder, request, auth.Principal{
			ProjectID:     projectID,
			EnvironmentID: envID,
		})
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	hub.Broadcast(StreamEvent{
		Type:          "log",
		ProjectID:     projectID,
		EnvironmentID: envID,
		Payload:       "test-payload",
	})

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("stream handler did not terminate after cancel")
	}

	contentType := recorder.Header().Get("Content-Type")
	if contentType != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %s", contentType)
	}

	body := recorder.Body.String()
	if len(body) == 0 {
		t.Fatal("expected non-empty stream response")
	}
}
