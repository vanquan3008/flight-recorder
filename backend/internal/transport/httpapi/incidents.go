package httpapi

import (
	"context"
	"errors"
	"net/http"

	incidentapp "github.com/vanquan3008/tracepilot/internal/application/incident"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	replaydomain "github.com/vanquan3008/tracepilot/internal/domain/replay"
)

func (server *Server) listIncidents(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	filter, cursor, parseError := parseIncidentFilter(request)
	if parseError != nil {
		writeInvalidQuery(responseWriter, parseError)
		return
	}
	filter.Before, filter.CursorID = cursor.At, cursor.ID
	filter.EnvironmentID = principal.EnvironmentID
	incidents, queryError := server.backend.ListIncidents(request.Context(), principal.ProjectID, filter)
	if queryError != nil {
		server.writeInternalError(responseWriter, "list incidents", queryError)
		return
	}
	writePage(responseWriter, incidents, filter.Limit, func(record incident.Incident) pageCursor {
		return pageCursor{At: record.LastSeenAt, ID: record.ID}
	})
}

func (server *Server) getIncident(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	record, ok := server.loadIncident(responseWriter, request, principal)
	if !ok {
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, record)
}

func (server *Server) listIncidentOccurrences(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	writeIncidentPage(server, responseWriter, request, principal, "list incident occurrences",
		func(ctx context.Context, incidentID string, limit int, cursor pageCursor) ([]incident.Occurrence, error) {
			return server.backend.ListIncidentOccurrences(ctx, principal.ProjectID, principal.EnvironmentID, incidentID, limit, cursor.At, cursor.ID)
		},
		func(occurrence incident.Occurrence) pageCursor {
			return pageCursor{At: occurrence.OccurredAt, ID: occurrence.ID}
		})
}

func (server *Server) listIncidentEvents(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	writeIncidentPage(server, responseWriter, request, principal, "list incident events",
		func(ctx context.Context, incidentID string, limit int, cursor pageCursor) ([]incident.Event, error) {
			return server.backend.ListIncidentEvents(ctx, principal.ProjectID, principal.EnvironmentID, incidentID, limit, cursor.At, cursor.ID)
		},
		func(event incident.Event) pageCursor { return pageCursor{At: event.OccurredAt, ID: event.EventID} })
}

func (server *Server) listIncidentStatusChanges(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	writeIncidentPage(server, responseWriter, request, principal, "list incident status changes",
		func(ctx context.Context, incidentID string, limit int, cursor pageCursor) ([]incident.StatusChange, error) {
			return server.backend.ListIncidentStatusChanges(ctx, principal.ProjectID, principal.EnvironmentID, incidentID, limit, cursor.At, cursor.ID)
		},
		func(change incident.StatusChange) pageCursor { return pageCursor{At: change.ChangedAt, ID: change.ID} })
}

func (server *Server) listAllowedRequestExecutions(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	writeIncidentPage(server, responseWriter, request, principal, "list allowed request executions",
		func(ctx context.Context, incidentID string, limit int, cursor pageCursor) ([]replaydomain.Execution, error) {
			return server.backend.ListReplayExecutions(ctx, principal.ProjectID, principal.EnvironmentID, incidentID, replaydomain.ExecutionFilter{
				Before: cursor.At, CursorID: cursor.ID, Limit: limit,
			})
		},
		func(execution replaydomain.Execution) pageCursor {
			return pageCursor{At: execution.CreatedAt, ID: execution.ID}
		})
}

func (server *Server) getAllowedRequestExecution(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	record, ok := server.loadIncident(responseWriter, request, principal)
	if !ok {
		return
	}
	executionID := request.PathValue("executionID")
	if !requireUUID(responseWriter, executionID) {
		return
	}
	execution, queryError := server.backend.GetReplayExecution(request.Context(), principal.ProjectID, principal.EnvironmentID, record.ID, executionID)
	if queryError != nil {
		server.writeInternalError(responseWriter, "get allowed request execution", queryError)
		return
	}
	if execution.ID == "" {
		writeNotFound(responseWriter, "allowed request execution")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, execution)
}

func (server *Server) updateIncident(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	incidentID := request.PathValue("incidentID")
	if !requireUUID(responseWriter, incidentID) {
		return
	}
	var payload struct {
		Status string `json:"status"`
	}
	if decodeError := decodeJSONBody(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", "request body must contain a status")
		return
	}
	record, updateError := server.incidentCommands.UpdateStatus(request.Context(), incidentapp.UpdateStatusCommand{
		ProjectID: principal.ProjectID, EnvironmentID: principal.EnvironmentID, IncidentID: incidentID, Status: payload.Status, Actor: principal.Actor(),
	})
	switch {
	case errors.Is(updateError, incidentapp.ErrInvalidStatus):
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", "incident status is invalid")
	case errors.Is(updateError, incident.ErrNotFound):
		writeNotFound(responseWriter, "incident")
	case errors.Is(updateError, incident.ErrInvalidTransition):
		writeAPIError(responseWriter, http.StatusConflict, "INVALID_STATUS_TRANSITION", "incident status transition is not allowed")
	case updateError != nil:
		server.writeInternalError(responseWriter, "update incident", updateError)
	default:
		writeJSONValue(responseWriter, http.StatusOK, record)
	}
}

// loadIncident resolves the incidentID path value within the caller's project
// and this server's environment, answering 400 or 404 itself when it cannot.
func (server *Server) loadIncident(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) (incident.Incident, bool) {
	incidentID := request.PathValue("incidentID")
	if !requireUUID(responseWriter, incidentID) {
		return incident.Incident{}, false
	}
	record, queryError := server.backend.GetIncident(request.Context(), principal.ProjectID, principal.EnvironmentID, incidentID)
	if queryError != nil {
		server.writeInternalError(responseWriter, "get incident", queryError)
		return incident.Incident{}, false
	}
	if record.ID == "" {
		writeNotFound(responseWriter, "incident")
		return incident.Incident{}, false
	}
	return record, true
}

// writeIncidentPage serves a paginated list that belongs to one incident: the
// incident must exist for the caller before its children are listed.
func writeIncidentPage[T any](
	server *Server, responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal, operation string,
	list func(ctx context.Context, incidentID string, limit int, cursor pageCursor) ([]T, error),
	cursorOf func(T) pageCursor,
) {
	record, ok := server.loadIncident(responseWriter, request, principal)
	if !ok {
		return
	}
	limit, cursor, parseError := parsePagination(request, validUUID)
	if parseError != nil {
		writeInvalidQuery(responseWriter, parseError)
		return
	}
	items, queryError := list(request.Context(), record.ID, limit, cursor)
	if queryError != nil {
		server.writeInternalError(responseWriter, operation, queryError)
		return
	}
	writePage(responseWriter, items, limit, cursorOf)
}
