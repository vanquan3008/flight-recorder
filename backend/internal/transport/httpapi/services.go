package httpapi

import (
	"net/http"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

func (server *Server) listServices(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	from, fromError := parseTime(request.URL.Query().Get("from"))
	if fromError != nil {
		writeInvalidQuery(responseWriter, fromError)
		return
	}
	to, toError := parseTime(request.URL.Query().Get("to"))
	if toError != nil {
		writeInvalidQuery(responseWriter, toError)
		return
	}
	if rangeError := validateTimeRange(from, to); rangeError != nil {
		writeInvalidQuery(responseWriter, rangeError)
		return
	}
	services, queryError := server.backend.ListServices(request.Context(), principal.ProjectID, principal.EnvironmentID, from, to)
	if queryError != nil {
		server.writeInternalError(responseWriter, "list services", queryError)
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"data": services})
}

func (server *Server) listServiceDependencies(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	service := request.PathValue("service")
	dependencies, queryError := server.backend.ListServiceDependencies(request.Context(), principal.ProjectID, principal.EnvironmentID, service)
	if queryError != nil {
		server.writeInternalError(responseWriter, "list service dependencies", queryError)
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"service": service, "dependencies": dependencies})
}
