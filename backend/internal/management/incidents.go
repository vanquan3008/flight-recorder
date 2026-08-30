package management

import (
	"errors"
	"net/http"

	"github.com/vanquan3008/tracepilot/internal/postgres"
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
	incidents, queryError := server.database.ListIncidents(request.Context(), principal.ProjectID, filter)
	if queryError != nil {
		server.logger.Error("list incidents", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(incidents, filter.Limit, func(incident postgres.Incident) pageCursor {
		return pageCursor{At: incident.LastSeenAt, ID: incident.ID}
	}))
}

func (server *Server) getIncident(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	incident, queryError := server.database.GetIncident(request.Context(), principal.ProjectID, server.environmentID, request.PathValue("incidentID"))
	if queryError != nil {
		server.logger.Error("get incident", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	if incident.ID == "" {
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "incident not found")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, incident)
}

func (server *Server) listIncidentOccurrences(responseWriter http.ResponseWriter, request *http.Request) {
	principal, incident, ok := server.authenticateIncidentRead(responseWriter, request)
	if !ok {
		return
	}
	limit, parseError := parsePageSize(request.URL.Query().Get("limit"))
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	cursor, parseError := decodeCursor(request.URL.Query().Get("cursor"))
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	if parseError := validateCursorID(cursor, validUUID); parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	occurrences, queryError := server.database.ListIncidentOccurrences(request.Context(), principal.ProjectID, server.environmentID, incident.ID, limit, cursor.At, cursor.ID)
	if queryError != nil {
		server.logger.Error("list incident occurrences", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(occurrences, limit, func(occurrence postgres.IncidentOccurrence) pageCursor {
		return pageCursor{At: occurrence.OccurredAt, ID: occurrence.ID}
	}))
}

func (server *Server) listIncidentEvents(responseWriter http.ResponseWriter, request *http.Request) {
	principal, incident, ok := server.authenticateIncidentRead(responseWriter, request)
	if !ok {
		return
	}
	limit, parseError := parsePageSize(request.URL.Query().Get("limit"))
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	cursor, parseError := decodeCursor(request.URL.Query().Get("cursor"))
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	if parseError := validateCursorID(cursor, validUUID); parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	events, queryError := server.database.ListIncidentEvents(request.Context(), principal.ProjectID, server.environmentID, incident.ID, limit, cursor.At, cursor.ID)
	if queryError != nil {
		server.logger.Error("list incident events", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(events, limit, func(event postgres.IncidentEvent) pageCursor {
		return pageCursor{At: event.OccurredAt, ID: event.EventID}
	}))
}

func (server *Server) listIncidentStatusChanges(responseWriter http.ResponseWriter, request *http.Request) {
	principal, incident, ok := server.authenticateIncidentRead(responseWriter, request)
	if !ok {
		return
	}
	limit, parseError := parsePageSize(request.URL.Query().Get("limit"))
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	cursor, parseError := decodeCursor(request.URL.Query().Get("cursor"))
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	if parseError := validateCursorID(cursor, validUUID); parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	changes, queryError := server.database.ListIncidentStatusChanges(request.Context(), principal.ProjectID, server.environmentID, incident.ID, limit, cursor.At, cursor.ID)
	if queryError != nil {
		server.logger.Error("list incident status changes", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(changes, limit, func(change postgres.IncidentStatusChange) pageCursor {
		return pageCursor{At: change.ChangedAt, ID: change.ID}
	}))
}

func (server *Server) listAllowedRequestExecutions(responseWriter http.ResponseWriter, request *http.Request) {
	principal, incident, ok := server.authenticateIncidentRead(responseWriter, request)
	if !ok {
		return
	}
	limit, parseError := parsePageSize(request.URL.Query().Get("limit"))
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	cursor, parseError := decodeCursor(request.URL.Query().Get("cursor"))
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	if parseError := validateCursorID(cursor, validUUID); parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	executions, queryError := server.database.ListReplayExecutions(request.Context(), principal.ProjectID, server.environmentID, incident.ID, postgres.ReplayExecutionFilter{
		Before: cursor.At, CursorID: cursor.ID, Limit: limit,
	})
	if queryError != nil {
		server.logger.Error("list allowed request executions", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(executions, limit, func(execution postgres.ReplayExecution) pageCursor {
		return pageCursor{At: execution.CreatedAt, ID: execution.ID}
	}))
}

func (server *Server) getAllowedRequestExecution(responseWriter http.ResponseWriter, request *http.Request) {
	principal, incident, ok := server.authenticateIncidentRead(responseWriter, request)
	if !ok {
		return
	}
	if !requireUUID(responseWriter, request.PathValue("executionID")) {
		return
	}
	execution, queryError := server.database.GetReplayExecution(request.Context(), principal.ProjectID, server.environmentID, incident.ID, request.PathValue("executionID"))
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
	var payload struct {
		Status string `json:"status"`
	}
	if decodeError := decodeJSONBody(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", "request body must contain a status")
		return
	}
	incident, updateError := server.database.UpdateIncidentStatus(request.Context(), principal.ProjectID, server.environmentID, request.PathValue("incidentID"), payload.Status, principal.KeyID)
	switch {
	case errors.Is(updateError, postgres.ErrIncidentNotFound):
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "incident not found")
	case errors.Is(updateError, postgres.ErrInvalidIncidentTransition):
		writeAPIError(responseWriter, http.StatusConflict, "INVALID_STATUS_TRANSITION", "incident status transition is not allowed")
	case updateError != nil:
		server.logger.Error("update incident", "error", updateError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	default:
		writeJSONValue(responseWriter, http.StatusOK, incident)
	}
}

func (server *Server) authenticateIncidentRead(responseWriter http.ResponseWriter, request *http.Request) (postgres.Principal, postgres.Incident, bool) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return postgres.Principal{}, postgres.Incident{}, false
	}
	if !requireUUID(responseWriter, request.PathValue("incidentID")) {
		return postgres.Principal{}, postgres.Incident{}, false
	}
	incident, queryError := server.database.GetIncident(request.Context(), principal.ProjectID, server.environmentID, request.PathValue("incidentID"))
	if queryError != nil {
		server.logger.Error("get incident", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return postgres.Principal{}, postgres.Incident{}, false
	}
	if incident.ID == "" {
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "incident not found")
		return postgres.Principal{}, postgres.Incident{}, false
	}
	return principal, incident, true
}
