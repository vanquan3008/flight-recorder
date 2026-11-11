package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

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
		case <-server.streamHub.Done():
			return
		}
	}
}
