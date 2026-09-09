package httpapi

import (
	"errors"
	"net/http"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	replaydomain "github.com/vanquan3008/tracepilot/internal/domain/replay"
)

func (server *Server) listIncidents(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	filter, cursor, parseError := parseIncidentFilter(request)
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	filter.Before, filter.CursorID = cursor.At, cursor.ID
	filter.EnvironmentID = server.environmentID
	incidents, queryError := server.backend.ListIncidents(request.Context(), principal.ProjectID, filter)
	if queryError != nil {
		server.logger.Error("list incidents", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(incidents, filter.Limit, func(record incident.Incident) pageCursor {
		return pageCursor{At: record.LastSeenAt, ID: record.ID}
	}))
}

func (server *Server) getIncident(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	if !requireUUID(responseWriter, request.PathValue("incidentID")) {
		return
	}
	record, queryError := server.backend.GetIncident(request.Context(), principal.ProjectID, server.environmentID, request.PathValue("incidentID"))
	if queryError != nil {
		server.logger.Error("get incident", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	if record.ID == "" {
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "incident not found")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, record)
}

func (server *Server) listIncidentOccurrences(responseWriter http.ResponseWriter, request *http.Request) {
	principal, record, ok := server.authenticateIncidentRead(responseWriter, request)
	if !ok {
		return
	}
	limit, cursor, parseError := parsePagination(request, validUUID)
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	occurrences, queryError := server.backend.ListIncidentOccurrences(request.Context(), principal.ProjectID, server.environmentID, record.ID, limit, cursor.At, cursor.ID)
	if queryError != nil {
		server.logger.Error("list incident occurrences", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(occurrences, limit, func(occurrence incident.Occurrence) pageCursor {
		return pageCursor{At: occurrence.OccurredAt, ID: occurrence.ID}
	}))
}

func (server *Server) listIncidentEvents(responseWriter http.ResponseWriter, request *http.Request) {
	principal, record, ok := server.authenticateIncidentRead(responseWriter, request)
	if !ok {
		return
	}
	limit, cursor, parseError := parsePagination(request, validUUID)
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	events, queryError := server.backend.ListIncidentEvents(request.Context(), principal.ProjectID, server.environmentID, record.ID, limit, cursor.At, cursor.ID)
	if queryError != nil {
		server.logger.Error("list incident events", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(events, limit, func(event incident.Event) pageCursor {
		return pageCursor{At: event.OccurredAt, ID: event.EventID}
	}))
}

func (server *Server) listIncidentStatusChanges(responseWriter http.ResponseWriter, request *http.Request) {
	principal, record, ok := server.authenticateIncidentRead(responseWriter, request)
	if !ok {
		return
	}
	limit, cursor, parseError := parsePagination(request, validUUID)
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	changes, queryError := server.backend.ListIncidentStatusChanges(request.Context(), principal.ProjectID, server.environmentID, record.ID, limit, cursor.At, cursor.ID)
	if queryError != nil {
		server.logger.Error("list incident status changes", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(changes, limit, func(change incident.StatusChange) pageCursor {
		return pageCursor{At: change.ChangedAt, ID: change.ID}
	}))
}

func (server *Server) listAllowedRequestExecutions(responseWriter http.ResponseWriter, request *http.Request) {
	principal, record, ok := server.authenticateIncidentRead(responseWriter, request)
	if !ok {
		return
	}
	limit, cursor, parseError := parsePagination(request, validUUID)
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	executions, queryError := server.backend.ListReplayExecutions(request.Context(), principal.ProjectID, server.environmentID, record.ID, replaydomain.ExecutionFilter{
		Before: cursor.At, CursorID: cursor.ID, Limit: limit,
	})
	if queryError != nil {
		server.logger.Error("list allowed request executions", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(executions, limit, func(execution replaydomain.Execution) pageCursor {
		return pageCursor{At: execution.CreatedAt, ID: execution.ID}
	}))
}

func (server *Server) getAllowedRequestExecution(responseWriter http.ResponseWriter, request *http.Request) {
	principal, record, ok := server.authenticateIncidentRead(responseWriter, request)
	if !ok {
		return
	}
	if !requireUUID(responseWriter, request.PathValue("executionID")) {
		return
	}
	execution, queryError := server.backend.GetReplayExecution(request.Context(), principal.ProjectID, server.environmentID, record.ID, request.PathValue("executionID"))
	if queryError != nil {
		server.logger.Error("get allowed request execution", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	if execution.ID == "" {
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "allowed request execution not found")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, execution)
}

func (server *Server) updateIncident(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateScope(responseWriter, request, "write")
	if !ok {
		return
	}
	if !requireUUID(responseWriter, request.PathValue("incidentID")) {
		return
	}
	var payload struct {
		Status string `json:"status"`
	}
	if decodeError := decodeJSONBody(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", "request body must contain a status")
		return
	}
	if _, valid := incident.NormalizeStatus(payload.Status); !valid {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", "incident status is invalid")
		return
	}
	record, updateError := server.backend.UpdateIncidentStatus(request.Context(), principal.ProjectID, server.environmentID, request.PathValue("incidentID"), payload.Status, principal.KeyID)
	switch {
	case errors.Is(updateError, incident.ErrNotFound):
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "incident not found")
	case errors.Is(updateError, incident.ErrInvalidTransition):
		writeAPIError(responseWriter, http.StatusConflict, "INVALID_STATUS_TRANSITION", "incident status transition is not allowed")
	case updateError != nil:
		server.logger.Error("update incident", "error", updateError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	default:
		writeJSONValue(responseWriter, http.StatusOK, record)
	}
}

func (server *Server) authenticateIncidentRead(responseWriter http.ResponseWriter, request *http.Request) (auth.Principal, incident.Incident, bool) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return auth.Principal{}, incident.Incident{}, false
	}
	if !requireUUID(responseWriter, request.PathValue("incidentID")) {
		return auth.Principal{}, incident.Incident{}, false
	}
	record, queryError := server.backend.GetIncident(request.Context(), principal.ProjectID, server.environmentID, request.PathValue("incidentID"))
	if queryError != nil {
		server.logger.Error("get incident", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return auth.Principal{}, incident.Incident{}, false
	}
	if record.ID == "" {
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "incident not found")
		return auth.Principal{}, incident.Incident{}, false
	}
	return principal, record, true
}
