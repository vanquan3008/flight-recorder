package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/vanquan3008/tracepilot/internal/replay"
	"github.com/vanquan3008/tracepilot/internal/replay/capture"
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
	outcome, runError := server.replay.Run(request.Context(), replay.Command{
		ProjectID: principal.ProjectID, EnvironmentID: server.environmentID, IncidentID: request.PathValue("incidentID"),
		ActorKeyID: principal.KeyID, RequestID: request.Header.Get(requestIDHeader), Request: captured,
	})
	if errors.Is(runError, replay.ErrIncidentNotFound) {
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "incident not found")
		return
	}
	if runError != nil {
		server.logger.Error("run replay", "error", runError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusCreated, outcome)
}
