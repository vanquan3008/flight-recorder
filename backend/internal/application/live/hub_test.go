package live

import (
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func TestStreamHubSubscribeAndBroadcast(t *testing.T) {
	hub := NewHub()
	projectID := "proj-1"
	envID := "env-1"

	events, unsubscribe, admitted := hub.Subscribe(projectID, envID)
	if !admitted {
		t.Fatal("stream subscription was rejected")
	}
	defer unsubscribe()

	testEvent := Event{
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
	hub := NewHub()
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
	hub := NewHub()
	events, unsubscribe, admitted := hub.Subscribe("project", "production-id")
	if !admitted {
		t.Fatal("stream subscription was rejected")
	}
	defer unsubscribe()

	hub.Broadcast(Event{ProjectID: "project", EnvironmentID: "staging-id"})
	hub.Broadcast(Event{ProjectID: "other-project", EnvironmentID: "production-id"})
	hub.Broadcast(Event{ProjectID: "project"})
	select {
	case event := <-events:
		t.Fatalf("received event from another or unknown environment: %+v", event)
	default:
	}
}

func TestSlowSubscriberDoesNotBlockIngestion(t *testing.T) {
	hub := NewHub()
	defer hub.Close()
	events, unsubscribe, admitted := hub.Subscribe("project", "environment")
	if !admitted {
		t.Fatal("stream subscription was rejected")
	}
	defer unsubscribe()
	done := make(chan struct{})
	go func() {
		for range 1000 {
			hub.Broadcast(Event{ProjectID: "project", EnvironmentID: "environment"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("an unread subscriber blocked telemetry ingestion")
	}
	if len(events) == 0 {
		t.Fatal("subscriber received no buffered telemetry")
	}
}

func TestUnsubscribeStopsDeliveryAndReleasesCapacity(t *testing.T) {
	hub := NewHub()
	defer hub.Close()
	events, stop, admitted := hub.Subscribe("project", "environment")
	if !admitted {
		t.Fatal("stream subscription was rejected")
	}
	stop()
	stop()
	hub.Broadcast(Event{ProjectID: "project", EnvironmentID: "environment"})
	select {
	case event := <-events:
		t.Fatalf("unsubscribed client received %+v", event)
	default:
	}
	for range maxSubscribers {
		if _, _, admitted := hub.Subscribe("project", "environment"); !admitted {
			t.Fatal("unsubscribe did not release capacity")
		}
	}
}

func TestStreamHubBoundsSubscribers(t *testing.T) {
	hub := NewHub()
	unsubscribe := make([]func(), 0, maxSubscribers)
	for range maxSubscribers {
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
	hub := NewHub()
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
