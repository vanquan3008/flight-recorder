// Package incidents serves incidents, their timelines and analysis, status changes, and request replay.
package incidents

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"

	incidentapp "github.com/vanquan3008/tracepilot/internal/application/incident"
	"github.com/vanquan3008/tracepilot/internal/application/replay"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	replaydomain "github.com/vanquan3008/tracepilot/internal/domain/replay"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

type Backend interface {
	incidentapp.StatusStore
	ListIncidents(context.Context, string, incident.Filter) ([]incident.Incident, error)
	GetIncident(context.Context, string, string, string) (incident.Incident, error)
	ListIncidentOccurrences(context.Context, string, string, string, int, time.Time, string) ([]incident.Occurrence, error)
	ListIncidentEvents(context.Context, string, string, string, int, time.Time, string) ([]incident.Event, error)
	ListIncidentStatusChanges(context.Context, string, string, string, int, time.Time, string) ([]incident.StatusChange, error)
	ListReplayExecutions(context.Context, string, string, string, replaydomain.ExecutionFilter) ([]replaydomain.Execution, error)
	GetReplayExecution(context.Context, string, string, string, string) (replaydomain.Execution, error)
}

// AnalysisBackend is optional; without it the incident analysis route answers 501.
type AnalysisBackend interface {
	GetIncidentAnalysis(context.Context, string, string, string) (analysis.IncidentAnalysis, error)
}

// ReplayRunner executes a captured HTTP request against a live target.
type ReplayRunner interface {
	Run(context.Context, replay.Command) (replay.Outcome, error)
}

type Handler struct {
	backend  Backend
	analysis AnalysisBackend
	commands *incidentapp.Service
	replay   ReplayRunner
	// maxReplayBytes bounds a replayed request body.
	maxReplayBytes int64
	errors         api.Errors
}

// New leaves the replay route out when replayRunner is nil.
func New(backend Backend, analysisBackend AnalysisBackend, replayRunner ReplayRunner, maxReplayBytes int64, logger *slog.Logger) *Handler {
	return &Handler{
		backend: backend, analysis: analysisBackend, commands: incidentapp.New(backend),
		replay: replayRunner, maxReplayBytes: maxReplayBytes, errors: api.Errors{Logger: logger},
	}
}

func (handler *Handler) Routes() []api.Route {
	routes := []api.Route{
		{Path: "/api/v1/incidents", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"incidents"}, OperationID: "listIncidents", Query: []string{"Limit", "Cursor", "From", "To", "Service", "Severity", "Status"}, Handler: handler.list, Response: openapi.Page[incident.Incident]()},
		{Path: "/api/v1/incidents/{incidentID}", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"incidents"}, OperationID: "getIncident", Handler: handler.get, Response: openapi.Object[incident.Incident]()},
		{Path: "/api/v1/incidents/{incidentID}/occurrences", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"incidents"}, OperationID: "listIncidentOccurrences", Query: []string{"Limit", "Cursor"}, Handler: handler.listOccurrences, Response: openapi.Page[incident.Occurrence]()},
		{Path: "/api/v1/incidents/{incidentID}/events", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"incidents"}, OperationID: "listIncidentEvents", Query: []string{"Limit", "Cursor"}, Handler: handler.listEvents, Response: openapi.Page[incident.Event]()},
		{Path: "/api/v1/incidents/{incidentID}/status-changes", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"incidents"}, OperationID: "listIncidentStatusChanges", Query: []string{"Limit", "Cursor"}, Handler: handler.listStatusChanges, Response: openapi.Page[incident.StatusChange]()},
		{Path: "/api/v1/incidents/{incidentID}/analysis", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"incidents", "analysis"}, OperationID: "getIncidentAnalysis", Handler: handler.getAnalysis, Response: openapi.Object[analysis.IncidentAnalysis]()},
		{Path: "/api/v1/incidents/{incidentID}/executions", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"replay"}, OperationID: "listReplayExecutions", Query: []string{"Limit", "Cursor"}, Handler: handler.listExecutions, Response: openapi.Page[replaydomain.Execution]()},
		{Path: "/api/v1/incidents/{incidentID}/executions/{executionID}", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"replay"}, OperationID: "getReplayExecution", Handler: handler.getExecution, Response: openapi.Object[replaydomain.Execution]()},
		{Path: "/api/v1/incidents/{incidentID}", Method: http.MethodPatch, Scope: auth.ScopeWrite, Tags: []string{"incidents"}, OperationID: "updateIncident", RequestBody: true, Handler: handler.update, Request: openapi.TypeOf[updateIncidentRequest](), Response: openapi.Object[incident.Incident]()},
	}
	if handler.replay != nil {
		routes = append(routes, api.Route{Path: "/api/v1/incidents/{incidentID}/replays", Method: http.MethodPost, Scope: auth.ScopeReplay, Tags: []string{"replay"}, OperationID: "replayIncident", RequestBody: true, Handler: handler.replayRequest, Request: openapi.TypeOf[replayRequest](), Response: openapi.Object[replay.Outcome]()})
	}
	return routes
}

type updateIncidentRequest struct {
	Status string `json:"status"`
}

func (handler *Handler) list(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	filter, cursor, parseError := parseFilter(request)
	if parseError != nil {
		render.WriteInvalidQuery(responseWriter, parseError)
		return
	}
	filter.Before, filter.CursorID = cursor.At, cursor.ID
	filter.EnvironmentID = principal.EnvironmentID
	incidents, queryError := handler.backend.ListIncidents(request.Context(), principal.ProjectID, filter)
	if queryError != nil {
		handler.errors.Internal(responseWriter, request, "list incidents", queryError)
		return
	}
	render.WritePage(responseWriter, incidents, filter.Limit, func(record incident.Incident) render.PageCursor {
		return render.PageCursor{At: record.LastSeenAt, ID: record.ID}
	})
}

func (handler *Handler) get(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if record, ok := handler.load(responseWriter, request, principal); ok {
		render.WriteJSON(responseWriter, http.StatusOK, record)
	}
}

func (handler *Handler) listOccurrences(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	writeChildPage(handler, responseWriter, request, principal, "list incident occurrences",
		func(ctx context.Context, incidentID string, page api.Page) ([]incident.Occurrence, error) {
			return handler.backend.ListIncidentOccurrences(ctx, principal.ProjectID, principal.EnvironmentID, incidentID, page.Limit, page.Before, page.CursorID)
		},
		func(occurrence incident.Occurrence) render.PageCursor {
			return render.PageCursor{At: occurrence.OccurredAt, ID: occurrence.ID}
		})
}

func (handler *Handler) listEvents(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	writeChildPage(handler, responseWriter, request, principal, "list incident events",
		func(ctx context.Context, incidentID string, page api.Page) ([]incident.Event, error) {
			return handler.backend.ListIncidentEvents(ctx, principal.ProjectID, principal.EnvironmentID, incidentID, page.Limit, page.Before, page.CursorID)
		},
		func(event incident.Event) render.PageCursor {
			return render.PageCursor{At: event.OccurredAt, ID: event.EventID}
		})
}

func (handler *Handler) listStatusChanges(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	writeChildPage(handler, responseWriter, request, principal, "list incident status changes",
		func(ctx context.Context, incidentID string, page api.Page) ([]incident.StatusChange, error) {
			return handler.backend.ListIncidentStatusChanges(ctx, principal.ProjectID, principal.EnvironmentID, incidentID, page.Limit, page.Before, page.CursorID)
		},
		func(change incident.StatusChange) render.PageCursor {
			return render.PageCursor{At: change.ChangedAt, ID: change.ID}
		})
}

func (handler *Handler) getAnalysis(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	incidentID := request.PathValue("incidentID")
	if !telemetry.ValidUUID(incidentID) {
		render.WriteAPIError(responseWriter, http.StatusBadRequest, "INVALID_ID", "incident ID must be a UUID")
		return
	}
	if handler.analysis == nil {
		render.WriteAPIError(responseWriter, http.StatusNotImplemented, "NOT_IMPLEMENTED", "observability analysis is unavailable")
		return
	}
	result, err := handler.analysis.GetIncidentAnalysis(request.Context(), principal.ProjectID, principal.EnvironmentID, incidentID)
	if err != nil {
		handler.errors.Internal(responseWriter, request, "get incident analysis", err)
		return
	}
	if result.IncidentID == "" {
		render.WriteNotFound(responseWriter, "incident")
		return
	}
	render.WriteJSON(responseWriter, http.StatusOK, result)
}

func (handler *Handler) update(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	incidentID := request.PathValue("incidentID")
	if !render.RequireUUID(responseWriter, incidentID) {
		return
	}
	var payload updateIncidentRequest
	if decodeError := render.DecodeJSONBody(request, &payload); decodeError != nil {
		render.WriteAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", "request body must contain a status")
		return
	}
	record, updateError := handler.commands.UpdateStatus(request.Context(), incidentapp.UpdateStatusCommand{
		ProjectID: principal.ProjectID, EnvironmentID: principal.EnvironmentID, IncidentID: incidentID, Status: payload.Status, Actor: principal.Actor(),
	})
	switch {
	case errors.Is(updateError, incidentapp.ErrInvalidStatus):
		render.WriteAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", "incident status is invalid")
	case errors.Is(updateError, incident.ErrNotFound):
		render.WriteNotFound(responseWriter, "incident")
	case errors.Is(updateError, incident.ErrInvalidTransition):
		render.WriteAPIError(responseWriter, http.StatusConflict, "INVALID_STATUS_TRANSITION", "incident status transition is not allowed")
	case updateError != nil:
		handler.errors.Internal(responseWriter, request, "update incident", updateError)
	default:
		render.WriteJSON(responseWriter, http.StatusOK, record)
	}
}

// load resolves the incidentID path value within the caller's project and environment, answering
// 400 or 404 itself when it cannot.
func (handler *Handler) load(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) (incident.Incident, bool) {
	incidentID := request.PathValue("incidentID")
	if !render.RequireUUID(responseWriter, incidentID) {
		return incident.Incident{}, false
	}
	record, queryError := handler.backend.GetIncident(request.Context(), principal.ProjectID, principal.EnvironmentID, incidentID)
	if queryError != nil {
		handler.errors.Internal(responseWriter, request, "get incident", queryError)
		return incident.Incident{}, false
	}
	if record.ID == "" {
		render.WriteNotFound(responseWriter, "incident")
		return incident.Incident{}, false
	}
	return record, true
}

// writeChildPage serves a paginated list that belongs to one incident: the incident must exist for
// the caller before its children are listed.
func writeChildPage[T any](
	handler *Handler, responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal, operation string,
	list func(ctx context.Context, incidentID string, page api.Page) ([]T, error),
	cursorOf func(T) render.PageCursor,
) {
	record, ok := handler.load(responseWriter, request, principal)
	if !ok {
		return
	}
	api.ListPage(responseWriter, request, func(page api.Page) ([]T, error) {
		return list(request.Context(), record.ID, page)
	}, cursorOf, func(queryError error) { handler.errors.Internal(responseWriter, request, operation, queryError) })
}

func parseFilter(request *http.Request) (incident.Filter, render.PageCursor, error) {
	values := request.URL.Query()
	limit, parseError := render.ParsePageSize(values.Get("limit"))
	if parseError != nil {
		return incident.Filter{}, render.PageCursor{}, parseError
	}
	from, to, parseError := render.ParseTimeRange(values)
	if parseError != nil {
		return incident.Filter{}, render.PageCursor{}, parseError
	}
	status := strings.ToUpper(strings.TrimSpace(values.Get("status")))
	if status != "" {
		if _, valid := incident.NormalizeStatus(status); !valid {
			return incident.Filter{}, render.PageCursor{}, errors.New("incident status is invalid")
		}
	}
	cursor, parseError := render.ParsePageCursor(values.Get("cursor"), telemetry.ValidUUID)
	if parseError != nil {
		return incident.Filter{}, render.PageCursor{}, parseError
	}
	return incident.Filter{
		Service: strings.TrimSpace(values.Get("service")), Severity: strings.ToUpper(strings.TrimSpace(values.Get("severity"))),
		Status: status, From: from, To: to, Limit: limit,
	}, cursor, nil
}
