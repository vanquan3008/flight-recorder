package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

const maxStreamSubscribers = 256

type StreamEvent struct {
	Type          string `json:"type"` // "log" | "incident"
	ProjectID     string `json:"project_id"`
	EnvironmentID string `json:"environment_id"`
	Payload       any    `json:"payload"`
}

type streamSubscriber struct {
	projectID     string
	environmentID string
	channel       chan StreamEvent
}

type StreamHub struct {
	mutex       sync.RWMutex
	subscribers map[string]*streamSubscriber
	nextID      uint64
	done        chan struct{}
	closed      bool
}

func NewStreamHub() *StreamHub {
	return &StreamHub{
		subscribers: make(map[string]*streamSubscriber),
		done:        make(chan struct{}),
	}
}

func (hub *StreamHub) Subscribe(projectID, environmentID string) (<-chan StreamEvent, func(), bool) {
	hub.mutex.Lock()
	if hub.closed || len(hub.subscribers) >= maxStreamSubscribers {
		hub.mutex.Unlock()
		return nil, nil, false
	}
	hub.nextID++
	subID := fmt.Sprint(hub.nextID)
	ch := make(chan StreamEvent, 64)
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

func (hub *StreamHub) Close() {
	hub.mutex.Lock()
	if !hub.closed {
		hub.closed = true
		close(hub.done)
	}
	hub.mutex.Unlock()
}

func (hub *StreamHub) Broadcast(event StreamEvent) {
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

func (hub *StreamHub) BroadcastEvents(events []telemetry.Event) {
	for _, event := range events {
		if event.Signal == telemetry.SignalLog || event.Type == telemetry.EventLog {
			hub.Broadcast(StreamEvent{
				Type:          "log",
				ProjectID:     event.ProjectID,
				EnvironmentID: event.EnvironmentID,
				Payload: map[string]any{
					"event_id":            event.ID,
					"service_name":        event.ServiceName,
					"occurred_at":         event.Timestamp.Format(time.RFC3339Nano),
					"severity_text":       event.Severity,
					"body":                event.LogBody,
					"resource_attributes": event.ResourceAttributes,
					"trace_id":            event.TraceID,
					"span_id":             event.SpanID,
				},
			})
		}
		if event.Status == "ERROR" || event.Severity == "ERROR" || event.ErrorType != "" {
			hub.Broadcast(StreamEvent{
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

func (server *Server) streamLiveTelemetry(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if server.streamHub == nil {
		writeAPIError(responseWriter, http.StatusServiceUnavailable, "STREAMING_DISABLED", "streaming hub is not initialized")
		return
	}

	events, unsubscribe, admitted := server.streamHub.Subscribe(principal.ProjectID, principal.EnvironmentID)
	if !admitted {
		responseWriter.Header().Set("Retry-After", "1")
		writeAPIError(responseWriter, http.StatusServiceUnavailable, "STREAM_LIMIT", "too many live streams are open")
		return
	}
	defer unsubscribe()

	controller := http.NewResponseController(responseWriter)
	if err := controller.SetWriteDeadline(time.Time{}); err != nil {
		writeAPIError(responseWriter, http.StatusInternalServerError, "STREAMING_UNSUPPORTED", "streaming is not supported")
		return
	}
	send := func(frame string) bool {
		if request.Context().Err() != nil || controller.SetWriteDeadline(time.Now().Add(5*time.Second)) != nil {
			return false
		}
		_, err := fmt.Fprint(responseWriter, frame)
		return err == nil && controller.Flush() == nil
	}

	responseWriter.Header().Set("Content-Type", "text/event-stream")
	responseWriter.Header().Set("Cache-Control", "no-cache, no-transform")
	responseWriter.Header().Set("Connection", "keep-alive")
	responseWriter.Header().Set("X-Accel-Buffering", "no")

	if !send("event: connected\ndata: {}\n\n") {
		return
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event, open := <-events:
			if !open {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if !send(fmt.Sprintf("data: %s\n\n", data)) {
				return
			}
		case <-ticker.C:
			if !send(": ping\n\n") {
				return
			}
		case <-request.Context().Done():
			return
		case <-server.streamHub.done:
			return
		}
	}
}
