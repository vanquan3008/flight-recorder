package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
)

var analysisRoutes = []apiRouteDefinition{
	{path: "/api/v1/performance", method: http.MethodGet, scope: "read", tags: []string{"performance"}, operationID: "getRequestPerformance", query: []string{"Limit", "From", "To", "Service", "Endpoint"}, handler: (*Server).requestPerformance, response: openapi.Object[query.RequestPerformance]()},
	{path: "/api/v1/topology", method: http.MethodGet, scope: "read", tags: []string{"analysis"}, operationID: "listTopology", query: []string{"From", "To", "Service"}, handler: (*Server).listTopology, response: openapi.Object[analysis.Topology]()},
	{path: "/api/v1/metrics/query", method: http.MethodGet, scope: "read", tags: []string{"analysis", "metrics"}, operationID: "listMetricPoints", query: []string{"Limit", "From", "To", "Service", "Metric"}, handler: (*Server).listMetricPoints, response: openapi.List[analysis.MetricPoint]()},
	{path: "/api/v1/telemetry/quality", method: http.MethodGet, scope: "read", tags: []string{"analysis"}, operationID: "getTelemetryQuality", query: []string{"From", "To", "Service"}, handler: (*Server).telemetryQuality, response: openapi.Object[analysis.Quality]()},
	{path: "/api/v1/services", method: http.MethodGet, scope: "read", tags: []string{"services"}, operationID: "listServices", query: []string{"From", "To"}, handler: (*Server).listServices, response: openapi.List[query.ServiceSummary]()},
	{path: "/api/v1/services/{service}/dependencies", method: http.MethodGet, scope: "read", tags: []string{"services"}, operationID: "listServiceDependencies", handler: (*Server).listServiceDependencies, response: openapi.Object[serviceDependenciesResponse]()},
}

func (server *Server) analysisBackend(responseWriter http.ResponseWriter) (AnalysisBackend, bool) {
	backend, ok := server.backend.(AnalysisBackend)
	if !ok {
		writeAPIError(responseWriter, http.StatusNotImplemented, "NOT_IMPLEMENTED", "observability analysis is unavailable")
	}
	return backend, ok
}

func parseAnalysisFilter(request *http.Request) (analysis.Filter, error) {
	values := request.URL.Query()
	from, err := parseTime(values.Get("from"))
	if err != nil {
		return analysis.Filter{}, errors.New("time filters must use RFC3339")
	}
	to, err := parseTime(values.Get("to"))
	if err != nil {
		return analysis.Filter{}, errors.New("time filters must use RFC3339")
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-15 * time.Minute)
	}
	if err := validateTimeRange(from, to); err != nil {
		return analysis.Filter{}, err
	}
	if to.Sub(from) > 30*24*time.Hour {
		return analysis.Filter{}, errors.New("analysis time range must not exceed 30 days")
	}
	limit, err := parsePageSize(values.Get("limit"))
	if err != nil {
		return analysis.Filter{}, err
	}
	return analysis.Filter{EnvironmentID: "", Service: strings.TrimSpace(values.Get("service")), Metric: strings.TrimSpace(values.Get("metric")), From: from, To: to, Limit: limit}, nil
}

// analysisRequest resolves the analysis backend and the request's filter,
// scoped to this server's environment, answering the error itself when either
// is unavailable.
func (server *Server) analysisRequest(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) (AnalysisBackend, analysis.Filter, bool) {
	backend, ok := server.analysisBackend(responseWriter)
	if !ok {
		return nil, analysis.Filter{}, false
	}
	filter, err := parseAnalysisFilter(request)
	if err != nil {
		writeInvalidQuery(responseWriter, err)
		return nil, analysis.Filter{}, false
	}
	filter.EnvironmentID = principal.EnvironmentID
	return backend, filter, true
}

func (server *Server) listTopology(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	backend, filter, ok := server.analysisRequest(responseWriter, request, principal)
	if !ok {
		return
	}
	topology, err := backend.ListTopology(request.Context(), principal.ProjectID, filter)
	if err != nil {
		server.writeInternalError(responseWriter, "list topology", err)
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, topology)
}

func (server *Server) listMetricPoints(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	backend, filter, ok := server.analysisRequest(responseWriter, request, principal)
	if !ok {
		return
	}
	points, err := backend.ListMetricPoints(request.Context(), principal.ProjectID, filter)
	if err != nil {
		server.writeInternalError(responseWriter, "list metric points", err)
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"data": points})
}

func (server *Server) telemetryQuality(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	backend, filter, ok := server.analysisRequest(responseWriter, request, principal)
	if !ok {
		return
	}
	quality, err := backend.TelemetryQuality(request.Context(), principal.ProjectID, filter)
	if err != nil {
		server.writeInternalError(responseWriter, "telemetry quality", err)
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, quality)
}

func (server *Server) getIncidentAnalysis(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	incidentID := request.PathValue("incidentID")
	if !validUUID(incidentID) {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_ID", "incident ID must be a UUID")
		return
	}
	backend, ok := server.analysisBackend(responseWriter)
	if !ok {
		return
	}
	result, err := backend.GetIncidentAnalysis(request.Context(), principal.ProjectID, principal.EnvironmentID, incidentID)
	if err != nil {
		server.writeInternalError(responseWriter, "get incident analysis", err)
		return
	}
	if result.IncidentID == "" {
		writeNotFound(responseWriter, "incident")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, result)
}

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
	writeJSONValue(responseWriter, http.StatusOK, serviceDependenciesResponse{Service: service, Dependencies: dependencies})
}

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
