package httpapi

import (
	"net/http"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/query"
)

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
	record, queryError := server.backend.GetLog(request.Context(), principal.ProjectID, server.environmentID, eventID)
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
	filter.EnvironmentID = server.environmentID
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
