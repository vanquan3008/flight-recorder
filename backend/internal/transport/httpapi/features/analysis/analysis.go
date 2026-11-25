// Package analysis serves topology, metrics, telemetry quality, services, and request performance.
package analysis

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

const maxRange = 30 * 24 * time.Hour

type Backend interface {
	ListServices(context.Context, string, string, time.Time, time.Time) ([]query.ServiceSummary, error)
	ListServiceDependencies(context.Context, string, string, string) ([]query.ServiceDependency, error)
}

// InsightsBackend is optional, so lightweight deployments and test fakes can serve the core API
// without pretending analysis data exists.
type InsightsBackend interface {
	ListTopology(context.Context, string, analysis.Filter) (analysis.Topology, error)
	ListMetricPoints(context.Context, string, analysis.Filter) ([]analysis.MetricPoint, error)
	TelemetryQuality(context.Context, string, analysis.Filter) (analysis.Quality, error)
}

// PerformanceBackend is optional for the same reason.
type PerformanceBackend interface {
	RequestPerformance(context.Context, string, query.PerformanceFilter) (query.RequestPerformance, error)
}

type Handler struct {
	backend     Backend
	insights    InsightsBackend
	performance PerformanceBackend
	errors      api.Errors
}

// New serves 501 for the routes whose optional backend is nil.
func New(backend Backend, insights InsightsBackend, performance PerformanceBackend, logger *slog.Logger) *Handler {
	return &Handler{backend: backend, insights: insights, performance: performance, errors: api.Errors{Logger: logger}}
}

func (handler *Handler) Routes() []api.Route {
	return []api.Route{
		{Path: "/api/v1/performance", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"performance"}, OperationID: "getRequestPerformance", Query: []string{"Limit", "From", "To", "Service", "Endpoint"}, Handler: handler.requestPerformance, Response: openapi.Object[query.RequestPerformance]()},
		{Path: "/api/v1/topology", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"analysis"}, OperationID: "listTopology", Query: []string{"From", "To", "Service"}, Handler: handler.listTopology, Response: openapi.Object[analysis.Topology]()},
		{Path: "/api/v1/metrics/query", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"analysis", "metrics"}, OperationID: "listMetricPoints", Query: []string{"Limit", "From", "To", "Service", "Metric"}, Handler: handler.listMetricPoints, Response: openapi.List[analysis.MetricPoint]()},
		{Path: "/api/v1/telemetry/quality", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"analysis"}, OperationID: "getTelemetryQuality", Query: []string{"From", "To", "Service"}, Handler: handler.telemetryQuality, Response: openapi.Object[analysis.Quality]()},
		{Path: "/api/v1/services", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"services"}, OperationID: "listServices", Query: []string{"From", "To"}, Handler: handler.listServices, Response: openapi.List[query.ServiceSummary]()},
		{Path: "/api/v1/services/{service}/dependencies", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"services"}, OperationID: "listServiceDependencies", Handler: handler.listServiceDependencies, Response: openapi.Object[serviceDependenciesResponse]()},
	}
}

type serviceDependenciesResponse struct {
	Dependencies []query.ServiceDependency `json:"dependencies"`
	Service      string                    `json:"service"`
}

// insightsRequest checks the insights backend and parses the filter, scoped to the caller's
// environment, answering the error itself when either is unavailable.
func (handler *Handler) insightsRequest(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) (analysis.Filter, bool) {
	if handler.insights == nil {
		render.WriteAPIError(responseWriter, http.StatusNotImplemented, "NOT_IMPLEMENTED", "observability analysis is unavailable")
		return analysis.Filter{}, false
	}
	filter, err := parseFilter(request)
	if err != nil {
		render.WriteInvalidQuery(responseWriter, err)
		return analysis.Filter{}, false
	}
	filter.EnvironmentID = principal.EnvironmentID
	return filter, true
}

func (handler *Handler) listTopology(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	filter, ok := handler.insightsRequest(responseWriter, request, principal)
	if !ok {
		return
	}
	topology, err := handler.insights.ListTopology(request.Context(), principal.ProjectID, filter)
	if err != nil {
		handler.errors.Internal(responseWriter, request, "list topology", err)
		return
	}
	render.WriteJSON(responseWriter, http.StatusOK, topology)
}

func (handler *Handler) listMetricPoints(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	filter, ok := handler.insightsRequest(responseWriter, request, principal)
	if !ok {
		return
	}
	points, err := handler.insights.ListMetricPoints(request.Context(), principal.ProjectID, filter)
	if err != nil {
		handler.errors.Internal(responseWriter, request, "list metric points", err)
		return
	}
	render.WriteJSON(responseWriter, http.StatusOK, map[string]any{"data": points})
}

func (handler *Handler) telemetryQuality(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	filter, ok := handler.insightsRequest(responseWriter, request, principal)
	if !ok {
		return
	}
	quality, err := handler.insights.TelemetryQuality(request.Context(), principal.ProjectID, filter)
	if err != nil {
		handler.errors.Internal(responseWriter, request, "telemetry quality", err)
		return
	}
	render.WriteJSON(responseWriter, http.StatusOK, quality)
}

func (handler *Handler) listServices(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	from, to, rangeError := render.ParseTimeRange(request.URL.Query())
	if rangeError != nil {
		render.WriteInvalidQuery(responseWriter, rangeError)
		return
	}
	services, queryError := handler.backend.ListServices(request.Context(), principal.ProjectID, principal.EnvironmentID, from, to)
	if queryError != nil {
		handler.errors.Internal(responseWriter, request, "list services", queryError)
		return
	}
	render.WriteJSON(responseWriter, http.StatusOK, map[string]any{"data": services})
}

func (handler *Handler) listServiceDependencies(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	service := request.PathValue("service")
	dependencies, queryError := handler.backend.ListServiceDependencies(request.Context(), principal.ProjectID, principal.EnvironmentID, service)
	if queryError != nil {
		handler.errors.Internal(responseWriter, request, "list service dependencies", queryError)
		return
	}
	render.WriteJSON(responseWriter, http.StatusOK, serviceDependenciesResponse{Service: service, Dependencies: dependencies})
}

func (handler *Handler) requestPerformance(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	parsed, err := parseFilter(request)
	if err != nil {
		render.WriteInvalidQuery(responseWriter, err)
		return
	}
	filter := query.PerformanceFilter{
		EnvironmentID: principal.EnvironmentID, Service: parsed.Service, Endpoint: strings.TrimSpace(request.URL.Query().Get("endpoint")),
		From: parsed.From, To: parsed.To, Limit: parsed.Limit,
	}
	if err := filter.Validate(); err != nil {
		render.WriteInvalidQuery(responseWriter, err)
		return
	}
	if handler.performance == nil {
		render.WriteAPIError(responseWriter, http.StatusNotImplemented, "NOT_IMPLEMENTED", "request performance is unavailable")
		return
	}
	result, err := handler.performance.RequestPerformance(request.Context(), principal.ProjectID, filter)
	if err != nil {
		handler.errors.Internal(responseWriter, request, "request performance", err)
		return
	}
	render.WriteJSON(responseWriter, http.StatusOK, result)
}

// parseFilter reads the time range, defaulting to the last 15 minutes, and the service, metric, and
// limit filters.
func parseFilter(request *http.Request) (analysis.Filter, error) {
	values := request.URL.Query()
	from, err := render.ParseTime(values.Get("from"))
	if err != nil {
		return analysis.Filter{}, err
	}
	to, err := render.ParseTime(values.Get("to"))
	if err != nil {
		return analysis.Filter{}, err
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-15 * time.Minute)
	}
	if err := render.ValidateTimeRange(from, to); err != nil {
		return analysis.Filter{}, err
	}
	if to.Sub(from) > maxRange {
		return analysis.Filter{}, errors.New("analysis time range must not exceed 30 days")
	}
	limit, err := render.ParsePageSize(values.Get("limit"))
	if err != nil {
		return analysis.Filter{}, err
	}
	return analysis.Filter{Service: strings.TrimSpace(values.Get("service")), Metric: strings.TrimSpace(values.Get("metric")), From: from, To: to, Limit: limit}, nil
}
