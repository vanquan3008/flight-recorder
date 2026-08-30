package management

import (
	"net/http"

	"github.com/vanquan3008/tracepilot/internal/postgres"
)

func (server *Server) listTraces(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	filter, cursor, parseError := parseTraceFilter(request)
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	filter.Before, filter.CursorID = cursor.At, cursor.ID
	traces, queryError := server.database.ListTraces(request.Context(), principal.ProjectID, filter)
	if queryError != nil {
		server.logger.Error("list traces", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(traces, filter.Limit, func(trace postgres.TraceSummary) pageCursor {
		return pageCursor{At: trace.CursorAt, ID: trace.TraceID}
	}))
}

func (server *Server) getTrace(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	if !requireTraceID(responseWriter, request.PathValue("traceID")) {
		return
	}
	spans, queryError := server.database.GetTrace(request.Context(), principal.ProjectID, server.environmentID, request.PathValue("traceID"))
	if queryError != nil {
		server.logger.Error("get trace", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	if len(spans) == 0 {
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "trace not found")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"trace_id": request.PathValue("traceID"), "spans": spans})
}

func (server *Server) listTraceLogs(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	if !requireTraceID(responseWriter, request.PathValue("traceID")) {
		return
	}
	filter, cursor, parseError := parseLogFilter(request)
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	filter.TraceID = request.PathValue("traceID")
	filter.EnvironmentID = server.environmentID
	filter.Before, filter.CursorID = cursor.At, cursor.ID
	logs, queryError := server.database.ListLogs(request.Context(), principal.ProjectID, filter)
	if queryError != nil {
		server.logger.Error("list trace logs", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(logs, filter.Limit, func(record postgres.LogRecord) pageCursor {
		return pageCursor{At: record.OccurredAt, ID: record.EventID}
	}))
}
