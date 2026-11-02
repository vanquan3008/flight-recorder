package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/vanquan3008/tracepilot/internal/application/replay"
	"github.com/vanquan3008/tracepilot/internal/application/replay/capture"
	"github.com/vanquan3008/tracepilot/internal/auth"
)

func (server *Server) replayIncident(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	incidentID := request.PathValue("incidentID")
	if !requireUUID(responseWriter, incidentID) {
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
		ProjectID: principal.ProjectID, EnvironmentID: principal.EnvironmentID, IncidentID: incidentID,
		Actor: principal.Actor(), RequestID: request.Header.Get(requestIDHeader), Request: captured,
	})
	if errors.Is(runError, replay.ErrIncidentNotFound) {
		writeNotFound(responseWriter, "incident")
		return
	}
	if runError != nil {
		server.writeInternalError(responseWriter, "run replay", runError)
		return
	}
	writeJSONValue(responseWriter, http.StatusCreated, outcome)
}
