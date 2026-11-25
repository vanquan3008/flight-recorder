// Package stream serves the live telemetry stream as Server-Sent Events.
package stream

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/vanquan3008/tracepilot/internal/application/live"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

const (
	pingInterval = 15 * time.Second
	writeTimeout = 5 * time.Second
)

type Handler struct {
	hub *live.Hub
}

// New answers 503 when hub is nil.
func New(hub *live.Hub) *Handler {
	return &Handler{hub: hub}
}

// Routes are streaming: the router watches the caller's access instead of timing the request out.
func (handler *Handler) Routes() []api.Route {
	return []api.Route{
		{Path: "/api/v1/live/stream", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"Telemetric Streaming"}, OperationID: "streamLiveTelemetry", SuccessStatus: http.StatusOK, Handler: handler.serve, Streaming: true},
	}
}

func (handler *Handler) serve(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if handler.hub == nil {
		render.WriteAPIError(responseWriter, http.StatusServiceUnavailable, "STREAMING_DISABLED", "streaming hub is not initialized")
		return
	}
	events, unsubscribe, admitted := handler.hub.Subscribe(principal.ProjectID, principal.EnvironmentID)
	if !admitted {
		responseWriter.Header().Set("Retry-After", "1")
		render.WriteAPIError(responseWriter, http.StatusServiceUnavailable, "STREAM_LIMIT", "too many live streams are open")
		return
	}
	defer unsubscribe()

	controller := http.NewResponseController(responseWriter)
	if err := controller.SetWriteDeadline(time.Time{}); err != nil {
		render.WriteAPIError(responseWriter, http.StatusInternalServerError, "STREAMING_UNSUPPORTED", "streaming is not supported")
		return
	}
	send := func(frame string) bool {
		if request.Context().Err() != nil || controller.SetWriteDeadline(time.Now().Add(writeTimeout)) != nil {
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
	ticker := time.NewTicker(pingInterval)
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
		case <-handler.hub.Done():
			return
		}
	}
}
