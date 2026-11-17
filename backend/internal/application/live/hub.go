// Package live distributes persisted telemetry to bounded, process-local subscribers.
package live

import (
	"fmt"
	"sync"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

const maxSubscribers = 256

type Event struct {
	Type          string `json:"type"` // "log" | "incident"
	ProjectID     string `json:"project_id"`
	EnvironmentID string `json:"environment_id"`
	Payload       any    `json:"payload"`
}

type streamSubscriber struct {
	projectID     string
	environmentID string
	channel       chan Event
}

type Hub struct {
	mutex       sync.RWMutex
	subscribers map[string]*streamSubscriber
	nextID      uint64
	done        chan struct{}
	closed      bool
}

func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[string]*streamSubscriber),
		done:        make(chan struct{}),
	}
}

func (hub *Hub) Subscribe(projectID, environmentID string) (<-chan Event, func(), bool) {
	hub.mutex.Lock()
	if hub.closed || len(hub.subscribers) >= maxSubscribers {
		hub.mutex.Unlock()
		return nil, nil, false
	}
	hub.nextID++
	subID := fmt.Sprint(hub.nextID)
	ch := make(chan Event, 64)
	hub.subscribers[subID] = &streamSubscriber{
		projectID: projectID, environmentID: environmentID, channel: ch,
	}
	hub.mutex.Unlock()

	unsubscribe := func() {
		hub.mutex.Lock()
		delete(hub.subscribers, subID)
		hub.mutex.Unlock()
	}

	return ch, unsubscribe, true
}

func (hub *Hub) Close() {
	hub.mutex.Lock()
	if !hub.closed {
		hub.closed = true
		close(hub.done)
	}
	hub.mutex.Unlock()
}

func (hub *Hub) Broadcast(event Event) {
	hub.mutex.RLock()
	defer hub.mutex.RUnlock()

	if hub.closed {
		return
	}
	for _, sub := range hub.subscribers {
		if sub.projectID == event.ProjectID && (sub.environmentID == "" || sub.environmentID == event.EnvironmentID) {
			select {
			case sub.channel <- event:
			default:
				// Skip if client is slow to avoid blocking telemetry ingestion
			}
		}
	}
}

func (hub *Hub) BroadcastEvents(events []telemetry.Event) {
	for _, event := range events {
		if event.Signal == telemetry.SignalLog || event.Type == telemetry.EventLog {
			hub.Broadcast(Event{
				Type:          "log",
				ProjectID:     event.ProjectID,
				EnvironmentID: event.EnvironmentID,
				Payload: map[string]any{
					"event_id":            event.ID,
					"service_name":        event.ServiceName,
					"occurred_at":         event.Timestamp.Format(time.RFC3339Nano),
					"severity_text":       event.Severity,
					"severity_number":     event.SeverityNumber,
					"body":                event.LogBody,
					"resource_attributes": event.ResourceAttributes,
					"trace_id":            event.TraceID,
					"span_id":             event.SpanID,
				},
			})
		}
		if incident.Candidate(event) {
			hub.Broadcast(Event{
				Type:          "incident",
				ProjectID:     event.ProjectID,
				EnvironmentID: event.EnvironmentID,
				Payload: map[string]any{
					"event_id":      event.ID,
					"service_name":  event.ServiceName,
					"title":         event.Operation,
					"severity":      "ERROR",
					"error_type":    event.ErrorType,
					"error_message": event.ErrorMessage,
					"occurred_at":   event.Timestamp.Format(time.RFC3339Nano),
					"trace_id":      event.TraceID,
				},
			})
		}
	}
}

func (hub *Hub) Done() <-chan struct{} { return hub.done }
