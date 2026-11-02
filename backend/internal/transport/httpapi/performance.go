package httpapi

import (
	"net/http"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/query"
)

func (server *Server) requestPerformance(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	parsed, err := parseAnalysisFilter(request)
	if err != nil {
		writeInvalidQuery(responseWriter, err)
		return
	}
	filter := query.PerformanceFilter{
		EnvironmentID: principal.EnvironmentID, Service: parsed.Service, Endpoint: strings.TrimSpace(request.URL.Query().Get("endpoint")),
		From: parsed.From, To: parsed.To, Limit: parsed.Limit,
	}
	if err := filter.Validate(); err != nil {
		writeInvalidQuery(responseWriter, err)
		return
	}
	backend, ok := server.backend.(PerformanceBackend)
	if !ok {
		writeAPIError(responseWriter, http.StatusNotImplemented, "NOT_IMPLEMENTED", "request performance is unavailable")
		return
	}
	result, err := backend.RequestPerformance(request.Context(), principal.ProjectID, filter)
	if err != nil {
		server.writeInternalError(responseWriter, "request performance", err)
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, result)
}
