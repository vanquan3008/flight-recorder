// Package traces serves trace search and a trace's spans page by page.
package traces

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

const (
	// MaxSpansPerPage caps one response. Larger traces are read page by page through next_cursor, so
	// no request holds a whole trace in memory.
	MaxSpansPerPage = 500
	// maxPageBytes ends a page early when its spans are unusually large.
	maxPageBytes = 8 << 20
)

type Backend interface {
	ListTraces(context.Context, string, query.TraceFilter) ([]query.TraceSummary, error)
	GetTrace(context.Context, string, string, string, query.SpanPage) ([]query.SpanRecord, bool, error)
}

type Handler struct {
	backend Backend
	errors  api.Errors
}

func New(backend Backend, logger *slog.Logger) *Handler {
	return &Handler{backend: backend, errors: api.Errors{Logger: logger}}
}

func (handler *Handler) Routes() []api.Route {
	return []api.Route{
		{Path: "/api/v1/traces", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"traces"}, OperationID: "listTraces", Query: []string{"Limit", "Cursor", "TraceListFrom", "To", "Service", "Operation", "Status", "TraceID", "MinDuration", "MaxDuration"}, Handler: handler.list, Response: openapi.Page[query.TraceSummary]()},
		{Path: "/api/v1/traces/{traceID}", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"traces"}, OperationID: "getTrace", Query: []string{"SpanLimit", "Cursor"}, Handler: handler.get, Response: openapi.Object[traceResponse]()},
	}
}

type traceResponse struct {
	NextCursor string             `json:"next_cursor,omitempty"`
	Spans      []query.SpanRecord `json:"spans"`
	TraceID    string             `json:"trace_id"`
}

func (handler *Handler) list(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	filter, cursor, parseError := parseFilter(request)
	if parseError != nil {
		render.WriteInvalidQuery(responseWriter, parseError)
		return
	}
	filter.EnvironmentID = principal.EnvironmentID
	filter.Before, filter.CursorID = cursor.At, cursor.ID
	if filter.From.IsZero() && filter.TraceID == "" {
		filter.From = render.WindowStart(filter.To, filter.Before, time.Now())
	}
	traces, queryError := handler.backend.ListTraces(request.Context(), principal.ProjectID, filter)
	if queryError != nil {
		handler.errors.Internal(responseWriter, request, "list traces", queryError)
		return
	}
	render.WritePage(responseWriter, traces, filter.Limit, func(trace query.TraceSummary) render.PageCursor {
		return render.PageCursor{At: trace.CursorAt, ID: trace.TraceID}
	})
}

// get serves one page of a trace's spans, oldest first. next_cursor is present exactly when more
// spans follow, so a client can always tell a complete trace from a partial one.
func (handler *Handler) get(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	traceID, ok := render.PathTraceID(responseWriter, request)
	if !ok {
		return
	}
	limit, cursor, parseError := parseSpanPage(request)
	if parseError != nil {
		render.WriteInvalidQuery(responseWriter, parseError)
		return
	}
	spans, hasMore, queryError := handler.backend.GetTrace(request.Context(), principal.ProjectID, principal.EnvironmentID, traceID, query.SpanPage{
		Limit: limit, MaxBytes: maxPageBytes, AfterStartedAt: cursor.At, AfterEventID: cursor.ID,
	})
	if queryError != nil {
		handler.errors.Internal(responseWriter, request, "get trace", queryError)
		return
	}
	if len(spans) == 0 && cursor.ID == "" {
		render.WriteNotFound(responseWriter, "trace")
		return
	}
	if spans == nil {
		spans = []query.SpanRecord{} // clients concatenate pages, so never send null
	}
	response := traceResponse{TraceID: traceID, Spans: spans}
	if hasMore {
		last := spans[len(spans)-1]
		response.NextCursor = render.EncodeCursor(render.PageCursor{At: last.StartedAt, ID: last.EventID})
	}
	render.WriteJSON(responseWriter, http.StatusOK, response)
}

// parseSpanPage reads limit (1..MaxSpansPerPage, default the maximum) and the cursor returned by the
// previous page.
func parseSpanPage(request *http.Request) (int, render.PageCursor, error) {
	limit := MaxSpansPerPage
	if value := request.URL.Query().Get("limit"); value != "" {
		parsed, parseError := strconv.Atoi(value)
		if parseError != nil || parsed < 1 || parsed > MaxSpansPerPage {
			return 0, render.PageCursor{}, fmt.Errorf("limit must be between 1 and %d", MaxSpansPerPage)
		}
		limit = parsed
	}
	cursor, parseError := render.ParsePageCursor(request.URL.Query().Get("cursor"), telemetry.ValidUUID)
	if parseError != nil {
		return 0, render.PageCursor{}, parseError
	}
	return limit, cursor, nil
}

func parseFilter(request *http.Request) (query.TraceFilter, render.PageCursor, error) {
	values := request.URL.Query()
	limit, parseError := render.ParsePageSize(values.Get("limit"))
	if parseError != nil {
		return query.TraceFilter{}, render.PageCursor{}, parseError
	}
	from, to, parseError := render.ParseTimeRange(values)
	if parseError != nil {
		return query.TraceFilter{}, render.PageCursor{}, parseError
	}
	minimumDuration, parseError := render.ParseDurationNanos(values.Get("minDuration"))
	if parseError != nil {
		return query.TraceFilter{}, render.PageCursor{}, parseError
	}
	maximumDuration, parseError := render.ParseDurationNanos(values.Get("maxDuration"))
	if parseError != nil {
		return query.TraceFilter{}, render.PageCursor{}, parseError
	}
	if minimumDuration != nil && maximumDuration != nil && *minimumDuration > *maximumDuration {
		return query.TraceFilter{}, render.PageCursor{}, errors.New("minDuration must not exceed maxDuration")
	}
	cursor, parseError := render.ParsePageCursor(values.Get("cursor"), render.ValidTraceID)
	if parseError != nil {
		return query.TraceFilter{}, render.PageCursor{}, parseError
	}
	cursor.ID = strings.ToLower(cursor.ID)
	traceID, parseError := render.QueryTraceID(values)
	if parseError != nil {
		return query.TraceFilter{}, render.PageCursor{}, parseError
	}
	return query.TraceFilter{Service: values.Get("service"), Operation: values.Get("operation"), Status: values.Get("status"), TraceID: traceID, From: from, To: to, MinDurationNs: minimumDuration, MaxDurationNs: maximumDuration, Limit: limit}, cursor, nil
}
