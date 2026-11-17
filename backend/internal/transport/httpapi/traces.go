package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
)

var traceRoutes = []apiRouteDefinition{
	{path: "/api/v1/traces", method: http.MethodGet, scope: "read", tags: []string{"traces"}, operationID: "listTraces", query: []string{"Limit", "Cursor", "TraceListFrom", "To", "Service", "Operation", "Status", "TraceID", "MinDuration", "MaxDuration"}, handler: (*Server).listTraces, response: openapi.Page[query.TraceSummary]()},
	{path: "/api/v1/traces/{traceID}", method: http.MethodGet, scope: "read", tags: []string{"traces"}, operationID: "getTrace", query: []string{"SpanLimit", "Cursor"}, handler: (*Server).getTrace, response: openapi.Object[traceResponse]()},
	{path: "/api/v1/traces/{traceID}/logs", method: http.MethodGet, scope: "read", tags: []string{"traces", "logs"}, operationID: "listTraceLogs", query: []string{"Limit", "Cursor", "From", "To", "Service", "LogSeverity", "SpanID", "Search"}, handler: (*Server).listTraceLogs, response: openapi.Page[query.LogRecord]()},
	{path: "/api/v1/logs", method: http.MethodGet, scope: "read", tags: []string{"logs"}, operationID: "listLogs", query: []string{"Limit", "Cursor", "From", "To", "Service", "LogSeverity", "TraceID", "SpanID", "Search"}, handler: (*Server).listLogs, response: openapi.Page[query.LogRecord]()},
	{path: "/api/v1/logs/{eventID}", method: http.MethodGet, scope: "read", tags: []string{"logs"}, operationID: "getLog", handler: (*Server).getLog, response: openapi.Object[query.LogRecord]()},
}

func (server *Server) listTraces(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	filter, cursor, parseError := parseTraceFilter(request)
	if parseError != nil {
		writeInvalidQuery(responseWriter, parseError)
		return
	}
	filter.EnvironmentID = principal.EnvironmentID
	filter.Before, filter.CursorID = cursor.At, cursor.ID
	applyDefaultTraceWindow(&filter, time.Now())
	traces, queryError := server.backend.ListTraces(request.Context(), principal.ProjectID, filter)
	if queryError != nil {
		server.writeInternalError(responseWriter, "list traces", queryError)
		return
	}
	writePage(responseWriter, traces, filter.Limit, func(trace query.TraceSummary) pageCursor {
		return pageCursor{At: trace.CursorAt, ID: trace.TraceID}
	})
}

// defaultTraceWindow bounds a trace list that names no start. Listing traces
// aggregates every span in its window, so an open-ended request would scan the
// whole environment; a trace ID lookup uses its own index and stays unbounded.
const defaultTraceWindow = time.Hour

// applyDefaultTraceWindow gives a request without `from` the hour before its
// end: `to` when given, else the page cursor, else now. Later pages therefore
// keep reading backwards one bounded window at a time.
func applyDefaultTraceWindow(filter *query.TraceFilter, now time.Time) {
	if !filter.From.IsZero() || filter.TraceID != "" {
		return
	}
	end := now
	switch {
	case !filter.To.IsZero():
		end = filter.To
	case !filter.Before.IsZero():
		end = filter.Before
	}
	filter.From = end.Add(-defaultTraceWindow)
}

const (
	// maxTraceSpansPerPage caps one response. Larger traces are read page by
	// page through next_cursor, so no request holds a whole trace in memory.
	maxTraceSpansPerPage = 500
	// maxTracePageBytes ends a page early when its spans are unusually large.
	maxTracePageBytes = 8 << 20
)

// getTrace serves one page of a trace's spans, oldest first. next_cursor is
// present exactly when more spans follow, so a client can always tell a
// complete trace from a partial one.
func (server *Server) getTrace(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	traceID := request.PathValue("traceID")
	if !requireTraceID(responseWriter, traceID) {
		return
	}
	limit, cursor, parseError := parseSpanPage(request)
	if parseError != nil {
		writeInvalidQuery(responseWriter, parseError)
		return
	}
	spans, hasMore, queryError := server.backend.GetTrace(request.Context(), principal.ProjectID, principal.EnvironmentID, traceID, query.SpanPage{
		Limit: limit, MaxBytes: maxTracePageBytes, AfterStartedAt: cursor.At, AfterEventID: cursor.ID,
	})
	if queryError != nil {
		server.writeInternalError(responseWriter, "get trace", queryError)
		return
	}
	if len(spans) == 0 && cursor.ID == "" {
		writeNotFound(responseWriter, "trace")
		return
	}
	if spans == nil {
		spans = []query.SpanRecord{} // clients concatenate pages, so never send null
	}
	response := traceResponse{TraceID: traceID, Spans: spans}
	if hasMore {
		last := spans[len(spans)-1]
		response.NextCursor = encodeCursor(pageCursor{At: last.StartedAt, ID: last.EventID})
	}
	writeJSONValue(responseWriter, http.StatusOK, response)
}

// parseSpanPage reads limit (1..maxTraceSpansPerPage, default the maximum) and
// the cursor returned by the previous page.
func parseSpanPage(request *http.Request) (int, pageCursor, error) {
	limit := maxTraceSpansPerPage
	if value := request.URL.Query().Get("limit"); value != "" {
		parsed, parseError := strconv.Atoi(value)
		if parseError != nil || parsed < 1 || parsed > maxTraceSpansPerPage {
			return 0, pageCursor{}, fmt.Errorf("limit must be between 1 and %d", maxTraceSpansPerPage)
		}
		limit = parsed
	}
	cursor, parseError := parsePageCursor(request.URL.Query().Get("cursor"), validUUID)
	if parseError != nil {
		return 0, pageCursor{}, parseError
	}
	return limit, cursor, nil
}

func (server *Server) listTraceLogs(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	traceID := request.PathValue("traceID")
	if !requireTraceID(responseWriter, traceID) {
		return
	}
	filter, cursor, parseError := parseLogFilter(request)
	if parseError != nil {
		writeInvalidQuery(responseWriter, parseError)
		return
	}
	filter.TraceID = traceID
	server.writeLogPage(responseWriter, request, principal, "list trace logs", filter, cursor)
}

func (server *Server) listLogs(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	filter, cursor, parseError := parseLogFilter(request)
	if parseError != nil {
		writeInvalidQuery(responseWriter, parseError)
		return
	}
	server.writeLogPage(responseWriter, request, principal, "list logs", filter, cursor)
}

func (server *Server) getLog(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	eventID := request.PathValue("eventID")
	if !requireUUID(responseWriter, eventID) {
		return
	}
	record, queryError := server.backend.GetLog(request.Context(), principal.ProjectID, principal.EnvironmentID, eventID)
	if queryError != nil {
		server.writeInternalError(responseWriter, "get log", queryError)
		return
	}
	if record.EventID == "" {
		writeNotFound(responseWriter, "log")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, record)
}

// writeLogPage scopes a parsed log filter to this server's environment and
// writes one page of matching logs.
func (server *Server) writeLogPage(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal, operation string, filter query.LogFilter, cursor pageCursor) {
	filter.EnvironmentID = principal.EnvironmentID
	filter.Before, filter.CursorID = cursor.At, cursor.ID
	logs, queryError := server.backend.ListLogs(request.Context(), principal.ProjectID, filter)
	if queryError != nil {
		server.writeInternalError(responseWriter, operation, queryError)
		return
	}
	writePage(responseWriter, logs, filter.Limit, func(record query.LogRecord) pageCursor {
		return pageCursor{At: record.OccurredAt, ID: record.EventID}
	})
}

func parseTraceFilter(request *http.Request) (query.TraceFilter, pageCursor, error) {
	values := request.URL.Query()
	limit, parseError := parsePageSize(values.Get("limit"))
	if parseError != nil {
		return query.TraceFilter{}, pageCursor{}, parseError
	}
	from, parseError := parseTime(values.Get("from"))
	if parseError != nil {
		return query.TraceFilter{}, pageCursor{}, parseError
	}
	to, parseError := parseTime(values.Get("to"))
	if parseError != nil {
		return query.TraceFilter{}, pageCursor{}, parseError
	}
	if rangeError := validateTimeRange(from, to); rangeError != nil {
		return query.TraceFilter{}, pageCursor{}, rangeError
	}
	minimumDuration, parseError := parseDurationNanos(values.Get("minDuration"))
	if parseError != nil {
		return query.TraceFilter{}, pageCursor{}, parseError
	}
	maximumDuration, parseError := parseDurationNanos(values.Get("maxDuration"))
	if parseError != nil {
		return query.TraceFilter{}, pageCursor{}, parseError
	}
	if minimumDuration != nil && maximumDuration != nil && *minimumDuration > *maximumDuration {
		return query.TraceFilter{}, pageCursor{}, errors.New("minDuration must not exceed maxDuration")
	}
	cursor, parseError := parsePageCursor(values.Get("cursor"), validTraceID)
	if parseError != nil {
		return query.TraceFilter{}, pageCursor{}, parseError
	}
	traceID := strings.TrimSpace(values.Get("traceId"))
	if traceID != "" && !validTraceID(traceID) {
		return query.TraceFilter{}, pageCursor{}, errors.New("traceId must be 16 bytes of hexadecimal")
	}
	return query.TraceFilter{Service: values.Get("service"), Operation: values.Get("operation"), Status: values.Get("status"), TraceID: traceID, From: from, To: to, MinDurationNs: minimumDuration, MaxDurationNs: maximumDuration, Limit: limit}, cursor, nil
}

func parseLogFilter(request *http.Request) (query.LogFilter, pageCursor, error) {
	values := request.URL.Query()
	limit, parseError := parsePageSize(values.Get("limit"))
	if parseError != nil {
		return query.LogFilter{}, pageCursor{}, parseError
	}
	from, parseError := parseTime(values.Get("from"))
	if parseError != nil {
		return query.LogFilter{}, pageCursor{}, parseError
	}
	to, parseError := parseTime(values.Get("to"))
	if parseError != nil {
		return query.LogFilter{}, pageCursor{}, parseError
	}
	if rangeError := validateTimeRange(from, to); rangeError != nil {
		return query.LogFilter{}, pageCursor{}, rangeError
	}
	search := strings.TrimSpace(values.Get("search"))
	if search != "" && (from.IsZero() || to.IsZero()) {
		return query.LogFilter{}, pageCursor{}, errors.New("log content search requires from and to")
	}
	if search != "" && to.Sub(from) > 24*time.Hour {
		return query.LogFilter{}, pageCursor{}, errors.New("log content search range must not exceed 24 hours")
	}
	cursor, parseError := parsePageCursor(values.Get("cursor"), validUUID)
	if parseError != nil {
		return query.LogFilter{}, pageCursor{}, parseError
	}
	traceID := strings.TrimSpace(values.Get("traceId"))
	if traceID != "" && !validTraceID(traceID) {
		return query.LogFilter{}, pageCursor{}, errors.New("traceId must be 16 bytes of hexadecimal")
	}
	spanID := strings.TrimSpace(values.Get("spanId"))
	if spanID != "" && !validSpanID(spanID) {
		return query.LogFilter{}, pageCursor{}, errors.New("spanId must be 8 bytes of hexadecimal")
	}
	return query.LogFilter{Service: values.Get("service"), Severity: values.Get("severity"), TraceID: traceID, SpanID: spanID, Search: search, From: from, To: to, Limit: limit}, cursor, nil
}

func validTraceID(value string) bool { return validHex(value, 32) }

func validSpanID(value string) bool { return validHex(value, 16) }

func requireTraceID(responseWriter http.ResponseWriter, value string) bool {
	if validTraceID(value) {
		return true
	}
	writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_PATH", "trace identifier must be 16 bytes of hexadecimal")
	return false
}
