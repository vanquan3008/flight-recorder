package httpapi

import "net/http"

func (server *Server) listServices(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	from, fromError := parseTime(request.URL.Query().Get("from"))
	to, toError := parseTime(request.URL.Query().Get("to"))
	if fromError != nil || toError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", "time filters must use RFC3339")
		return
	}
	if rangeError := validateTimeRange(from, to); rangeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", rangeError.Error())
		return
	}
	services, queryError := server.database.ListServices(request.Context(), principal.ProjectID, server.environmentID, from, to)
	if queryError != nil {
		server.logger.Error("list services", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"data": services})
}

func (server *Server) listServiceDependencies(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	dependencies, queryError := server.database.ListServiceDependencies(request.Context(), principal.ProjectID, server.environmentID, request.PathValue("service"))
	if queryError != nil {
		server.logger.Error("list service dependencies", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"service": request.PathValue("service"), "dependencies": dependencies})
}
