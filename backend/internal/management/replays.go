package management

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/vanquan3008/tracepilot/internal/replay"
	"github.com/vanquan3008/tracepilot/internal/replay/capture"
	"github.com/vanquan3008/tracepilot/internal/telemetry"
)

func (server *Server) replayIncident(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateScope(responseWriter, request, "replay")
	if !ok {
		return
	}
	if !requireUUID(responseWriter, request.PathValue("incidentID")) {
		return
	}
	var payload struct {
		Target  string            `json:"target"`
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`
		Body    json.RawMessage   `json:"body"`
	}
	if decodeError := decodeJSONBody(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", "request body must contain a target")
		return
	}
	headers := make(http.Header, len(payload.Headers))
	for name, value := range payload.Headers {
		headers.Set(name, value)
	}
	captured, captureError := capture.Sanitize(payload.Method, payload.Target, headers, payload.Body, server.replayMaxRequestBytes)
	if captureError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", captureError.Error())
		return
	}
	executionID := telemetry.DeterministicID(time.Now(), principal.KeyID, request.PathValue("incidentID"), request.Header.Get(requestIDHeader))
	started, startError := server.database.StartReplayExecution(request.Context(), executionID, principal.ProjectID, server.environmentID, request.PathValue("incidentID"), replay.AuditTarget(payload.Target), principal.KeyID)
	if startError != nil {
		server.logger.Error("start replay", "error", startError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	if !started {
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "incident not found")
		return
	}
	result := server.replay.Execute(request.Context(), captured)
	completionContext, cancelCompletion := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelCompletion()
	if completeError := server.database.CompleteReplayExecution(completionContext, executionID, principal.ProjectID, result); completeError != nil {
		server.logger.Error("complete replay", "error", completeError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusCreated, map[string]any{"id": executionID, "result": result})
}
