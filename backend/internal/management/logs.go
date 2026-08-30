package management

import (
	"net/http"

	"github.com/vanquan3008/tracepilot/internal/postgres"
)

func (server *Server) listLogs(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	filter, cursor, parseError := parseLogFilter(request)
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	filter.Before, filter.CursorID = cursor.At, cursor.ID
	filter.EnvironmentID = server.environmentID
	logs, queryError := server.database.ListLogs(request.Context(), principal.ProjectID, filter)
	if queryError != nil {
		server.logger.Error("list logs", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(logs, filter.Limit, func(record postgres.LogRecord) pageCursor {
		return pageCursor{At: record.OccurredAt, ID: record.EventID}
	}))
}

func (server *Server) getLog(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	if !requireUUID(responseWriter, request.PathValue("eventID")) {
		return
	}
	record, queryError := server.database.GetLog(request.Context(), principal.ProjectID, server.environmentID, request.PathValue("eventID"))
	if queryError != nil {
		server.logger.Error("get log", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	if record.EventID == "" {
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "log not found")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, record)
}
