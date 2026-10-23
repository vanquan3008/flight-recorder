package httpapi

import (
	"net/http"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/query"
)

func (server *Server) requestPerformance(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	parsed, err := parseAnalysisFilter(request)
	if err != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	filter := query.PerformanceFilter{
		EnvironmentID: server.environmentID, Service: parsed.Service, Endpoint: strings.TrimSpace(request.URL.Query().Get("endpoint")),
		From: parsed.From, To: parsed.To, Limit: parsed.Limit,
	}
	if err := filter.Validate(); err != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	backend, ok := server.backend.(PerformanceBackend)
	if !ok {
		writeAPIError(responseWriter, http.StatusNotImplemented, "NOT_IMPLEMENTED", "request performance is unavailable")
		return
	}
	result, err := backend.RequestPerformance(request.Context(), principal.ProjectID, filter)
	if err != nil {
		server.logger.Error("request performance", "error", err)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, result)
}
