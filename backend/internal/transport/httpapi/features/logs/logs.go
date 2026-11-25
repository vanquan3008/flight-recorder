// Package logs serves log search and the logs of one trace.
package logs

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

const (
	// maxSearchRange bounds a content search, which scans log bodies.
	maxSearchRange = 24 * time.Hour
)

type Backend interface {
	ListLogs(context.Context, string, query.LogFilter) ([]query.LogRecord, error)
	GetLog(context.Context, string, string, string) (query.LogRecord, error)
}

type Handler struct {
	backend Backend
	errors  api.Errors
}

func New(backend Backend, logger *slog.Logger) *Handler {
	return &Handler{backend: backend, errors: api.Errors{Logger: logger}}
}

// Routes are log queries, so they take slots from the log pool.
func (handler *Handler) Routes() []api.Route {
	return []api.Route{
		{Path: "/api/v1/traces/{traceID}/logs", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"traces", "logs"}, OperationID: "listTraceLogs", Query: []string{"Limit", "Cursor", "From", "To", "Service", "LogSeverity", "SpanID", "Search"}, Handler: handler.listForTrace, Response: openapi.Page[query.LogRecord](), LogQuery: true},
		{Path: "/api/v1/logs", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"logs"}, OperationID: "listLogs", Query: []string{"Limit", "Cursor", "LogListFrom", "To", "Service", "LogSeverity", "TraceID", "SpanID", "Search"}, Handler: handler.list, Response: openapi.Page[query.LogRecord](), LogQuery: true},
		{Path: "/api/v1/logs/{eventID}", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"logs"}, OperationID: "getLog", Handler: handler.get, Response: openapi.Object[query.LogRecord](), LogQuery: true},
	}
}

func (handler *Handler) listForTrace(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	traceID, ok := render.PathTraceID(responseWriter, request)
	if !ok {
		return
	}
	filter, cursor, parseError := parseFilter(request)
	if parseError != nil {
		render.WriteInvalidQuery(responseWriter, parseError)
		return
	}
	filter.TraceID = traceID
	handler.writePage(responseWriter, request, principal, "list trace logs", filter, cursor)
}

func (handler *Handler) list(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	filter, cursor, parseError := parseFilter(request)
	if parseError != nil {
		render.WriteInvalidQuery(responseWriter, parseError)
		return
	}
	handler.writePage(responseWriter, request, principal, "list logs", filter, cursor)
}

func (handler *Handler) get(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	eventID := request.PathValue("eventID")
	if !render.RequireUUID(responseWriter, eventID) {
		return
	}
	record, queryError := handler.backend.GetLog(request.Context(), principal.ProjectID, principal.EnvironmentID, eventID)
	if queryError != nil {
		handler.errors.Internal(responseWriter, request, "get log", queryError)
		return
	}
	if record.EventID == "" {
		render.WriteNotFound(responseWriter, "log")
		return
	}
	render.WriteJSON(responseWriter, http.StatusOK, record)
}

// writePage scopes a parsed filter to the caller's environment and writes one page of matching logs.
func (handler *Handler) writePage(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal, operation string, filter query.LogFilter, cursor render.PageCursor) {
	filter.EnvironmentID = principal.EnvironmentID
	filter.Before, filter.CursorID = cursor.At, cursor.ID
	if filter.From.IsZero() && filter.TraceID == "" {
		filter.From = render.WindowStart(filter.To, filter.Before, time.Now())
	}
	logs, queryError := handler.backend.ListLogs(request.Context(), principal.ProjectID, filter)
	if queryError != nil {
		handler.errors.Internal(responseWriter, request, operation, queryError)
		return
	}
	render.WritePage(responseWriter, logs, filter.Limit, func(record query.LogRecord) render.PageCursor {
		return render.PageCursor{At: record.OccurredAt, ID: record.EventID}
	})
}

func parseFilter(request *http.Request) (query.LogFilter, render.PageCursor, error) {
	values := request.URL.Query()
	limit, parseError := render.ParsePageSize(values.Get("limit"))
	if parseError != nil {
		return query.LogFilter{}, render.PageCursor{}, parseError
	}
	from, to, parseError := render.ParseTimeRange(values)
	if parseError != nil {
		return query.LogFilter{}, render.PageCursor{}, parseError
	}
	search := strings.TrimSpace(values.Get("search"))
	if search != "" && (from.IsZero() || to.IsZero()) {
		return query.LogFilter{}, render.PageCursor{}, errors.New("log content search requires from and to")
	}
	if search != "" && to.Sub(from) > maxSearchRange {
		return query.LogFilter{}, render.PageCursor{}, errors.New("log content search range must not exceed 24 hours")
	}
	cursor, parseError := render.ParsePageCursor(values.Get("cursor"), telemetry.ValidUUID)
	if parseError != nil {
		return query.LogFilter{}, render.PageCursor{}, parseError
	}
	traceID, parseError := render.QueryTraceID(values)
	if parseError != nil {
		return query.LogFilter{}, render.PageCursor{}, parseError
	}
	spanID, parseError := render.QuerySpanID(values)
	if parseError != nil {
		return query.LogFilter{}, render.PageCursor{}, parseError
	}
	return query.LogFilter{Service: values.Get("service"), Severity: values.Get("severity"), TraceID: traceID, SpanID: spanID, Search: search, From: from, To: to, Limit: limit}, cursor, nil
}
