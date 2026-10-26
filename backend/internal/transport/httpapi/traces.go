package httpapi

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/query"
)

func (server *Server) listTraces(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	filter, cursor, parseError := parseTraceFilter(request)
	if parseError != nil {
		writeInvalidQuery(responseWriter, parseError)
		return
	}
	filter.EnvironmentID = server.environmentID
	filter.Before, filter.CursorID = cursor.At, cursor.ID
	traces, queryError := server.backend.ListTraces(request.Context(), principal.ProjectID, filter)
	if queryError != nil {
		server.writeInternalError(responseWriter, "list traces", queryError)
		return
	}
	writePage(responseWriter, traces, filter.Limit, func(trace query.TraceSummary) pageCursor {
		return pageCursor{At: trace.CursorAt, ID: trace.TraceID}
	})
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
	spans, hasMore, queryError := server.backend.GetTrace(request.Context(), principal.ProjectID, server.environmentID, traceID, query.SpanPage{
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
	response := map[string]any{"trace_id": traceID, "spans": spans}
	if hasMore {
		last := spans[len(spans)-1]
		response["next_cursor"] = encodeCursor(pageCursor{At: last.StartedAt, ID: last.EventID})
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
