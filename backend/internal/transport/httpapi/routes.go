package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

// apiHandler serves one API route for a caller the router has already
// authenticated with the route's scope.
type apiHandler func(http.ResponseWriter, *http.Request, auth.Principal)

type apiRouteDefinition struct {
	path           string
	method         string
	scope          string
	tags           []string
	operationID    string
	successStatus  int
	requestBody    bool
	handler        func(*Server) apiHandler
	requiresReplay bool
}

// Keep the management router and OpenAPI contract sourced from one manifest.
// This prevents adding an endpoint to one surface while silently omitting it
// from the other.
var apiRouteDefinitions = []apiRouteDefinition{
	{path: "/api/v1/performance", method: http.MethodGet, scope: "read", tags: []string{"performance"}, operationID: "getRequestPerformance", handler: func(s *Server) apiHandler { return s.requestPerformance }},
	{path: "/api/v1/traces", method: http.MethodGet, scope: "read", tags: []string{"traces"}, operationID: "listTraces", handler: func(s *Server) apiHandler { return s.listTraces }},
	{path: "/api/v1/traces/{traceID}", method: http.MethodGet, scope: "read", tags: []string{"traces"}, operationID: "getTrace", handler: func(s *Server) apiHandler { return s.getTrace }},
	{path: "/api/v1/traces/{traceID}/logs", method: http.MethodGet, scope: "read", tags: []string{"traces", "logs"}, operationID: "listTraceLogs", handler: func(s *Server) apiHandler { return s.listTraceLogs }},
	{path: "/api/v1/logs", method: http.MethodGet, scope: "read", tags: []string{"logs"}, operationID: "listLogs", handler: func(s *Server) apiHandler { return s.listLogs }},
	{path: "/api/v1/logs/{eventID}", method: http.MethodGet, scope: "read", tags: []string{"logs"}, operationID: "getLog", handler: func(s *Server) apiHandler { return s.getLog }},
	{path: "/api/v1/topology", method: http.MethodGet, scope: "read", tags: []string{"analysis"}, operationID: "listTopology", handler: func(s *Server) apiHandler { return s.listTopology }},
	{path: "/api/v1/metrics/query", method: http.MethodGet, scope: "read", tags: []string{"analysis", "metrics"}, operationID: "listMetricPoints", handler: func(s *Server) apiHandler { return s.listMetricPoints }},
	{path: "/api/v1/telemetry/quality", method: http.MethodGet, scope: "read", tags: []string{"analysis"}, operationID: "getTelemetryQuality", handler: func(s *Server) apiHandler { return s.telemetryQuality }},
	{path: "/api/v1/services", method: http.MethodGet, scope: "read", tags: []string{"services"}, operationID: "listServices", handler: func(s *Server) apiHandler { return s.listServices }},
	{path: "/api/v1/services/{service}/dependencies", method: http.MethodGet, scope: "read", tags: []string{"services"}, operationID: "listServiceDependencies", handler: func(s *Server) apiHandler { return s.listServiceDependencies }},
	{path: "/api/v1/incidents", method: http.MethodGet, scope: "read", tags: []string{"incidents"}, operationID: "listIncidents", handler: func(s *Server) apiHandler { return s.listIncidents }},
	{path: "/api/v1/incidents/{incidentID}", method: http.MethodGet, scope: "read", tags: []string{"incidents"}, operationID: "getIncident", handler: func(s *Server) apiHandler { return s.getIncident }},
	{path: "/api/v1/incidents/{incidentID}/occurrences", method: http.MethodGet, scope: "read", tags: []string{"incidents"}, operationID: "listIncidentOccurrences", handler: func(s *Server) apiHandler { return s.listIncidentOccurrences }},
	{path: "/api/v1/incidents/{incidentID}/events", method: http.MethodGet, scope: "read", tags: []string{"incidents"}, operationID: "listIncidentEvents", handler: func(s *Server) apiHandler { return s.listIncidentEvents }},
	{path: "/api/v1/incidents/{incidentID}/status-changes", method: http.MethodGet, scope: "read", tags: []string{"incidents"}, operationID: "listIncidentStatusChanges", handler: func(s *Server) apiHandler { return s.listIncidentStatusChanges }},
	{path: "/api/v1/incidents/{incidentID}/analysis", method: http.MethodGet, scope: "read", tags: []string{"incidents", "analysis"}, operationID: "getIncidentAnalysis", handler: func(s *Server) apiHandler { return s.getIncidentAnalysis }},
	{path: "/api/v1/incidents/{incidentID}/executions", method: http.MethodGet, scope: "read", tags: []string{"replay"}, operationID: "listReplayExecutions", handler: func(s *Server) apiHandler { return s.listAllowedRequestExecutions }},
	{path: "/api/v1/incidents/{incidentID}/executions/{executionID}", method: http.MethodGet, scope: "read", tags: []string{"replay"}, operationID: "getReplayExecution", handler: func(s *Server) apiHandler { return s.getAllowedRequestExecution }},
	{path: "/api/v1/incidents/{incidentID}", method: http.MethodPatch, scope: "write", tags: []string{"incidents"}, operationID: "updateIncident", requestBody: true, handler: func(s *Server) apiHandler { return s.updateIncident }},
	{path: "/api/v1/incidents/{incidentID}/replays", method: http.MethodPost, scope: "replay", tags: []string{"replay"}, operationID: "replayIncident", requestBody: true, requiresReplay: true, handler: func(s *Server) apiHandler { return s.replayIncident }},
	{path: "/api/v1/api-keys", method: http.MethodGet, scope: "admin", tags: []string{"api-keys"}, operationID: "listAPIKeys", handler: func(s *Server) apiHandler { return s.listAPIKeys }},
	{path: "/api/v1/api-key-events", method: http.MethodGet, scope: "admin", tags: []string{"api-keys"}, operationID: "listAPIKeyEvents", handler: func(s *Server) apiHandler { return s.listAPIKeyEvents }},
	{path: "/api/v1/api-keys", method: http.MethodPost, scope: "admin", tags: []string{"api-keys"}, operationID: "createAPIKey", requestBody: true, handler: func(s *Server) apiHandler { return s.createAPIKey }},
	{path: "/api/v1/api-keys/{keyID}/rotate", method: http.MethodPost, scope: "admin", tags: []string{"api-keys"}, operationID: "rotateAPIKey", successStatus: http.StatusOK, handler: func(s *Server) apiHandler { return s.rotateAPIKey }},
	{path: "/api/v1/api-keys/{keyID}", method: http.MethodDelete, scope: "admin", tags: []string{"api-keys"}, operationID: "revokeAPIKey", handler: func(s *Server) apiHandler { return s.revokeAPIKey }},
	{path: "/api/v1/webhooks", method: http.MethodGet, scope: "admin", tags: []string{"webhooks"}, operationID: "listWebhooks", handler: func(s *Server) apiHandler { return s.listWebhooks }},
	{path: "/api/v1/webhooks", method: http.MethodPost, scope: "admin", tags: []string{"webhooks"}, operationID: "createWebhook", requestBody: true, handler: func(s *Server) apiHandler { return s.createWebhook }},
	{path: "/api/v1/webhooks/{webhookID}", method: http.MethodGet, scope: "admin", tags: []string{"webhooks"}, operationID: "getWebhook", handler: func(s *Server) apiHandler { return s.getWebhook }},
	{path: "/api/v1/webhooks/{webhookID}", method: http.MethodPut, scope: "admin", tags: []string{"webhooks"}, operationID: "updateWebhook", requestBody: true, handler: func(s *Server) apiHandler { return s.updateWebhook }},
	{path: "/api/v1/webhooks/{webhookID}", method: http.MethodDelete, scope: "admin", tags: []string{"webhooks"}, operationID: "deleteWebhook", handler: func(s *Server) apiHandler { return s.deleteWebhook }},
	{path: "/api/v1/webhooks/{webhookID}/rotate", method: http.MethodPost, scope: "admin", tags: []string{"webhooks"}, operationID: "rotateWebhookSecret", successStatus: http.StatusOK, handler: func(s *Server) apiHandler { return s.rotateWebhookSecret }},
	{path: "/api/v1/webhooks/{webhookID}/deliveries", method: http.MethodGet, scope: "admin", tags: []string{"webhooks"}, operationID: "listWebhookDeliveries", handler: func(s *Server) apiHandler { return s.listWebhookDeliveries }},
}

func (server *Server) registerAPIRoutes(router *http.ServeMux) {
	for _, route := range server.availableAPIRoutes() {
		router.HandleFunc(route.method+" "+route.path, server.requireScope(route.scope, route.handler(server)))
	}
}

// requireScope authenticates every API request against the scope its route
// declares, so handlers cannot forget the check or drift from OpenAPI. An
// authenticated request then takes one of a bounded number of query slots and
// runs under a deadline, so API load can neither take every database
// connection from ingestion nor hold one indefinitely.
func (server *Server) requireScope(scope string, handle apiHandler) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		principal, ok := server.authenticateScope(responseWriter, request, scope)
		if !ok {
			return
		}
		release, admitted := server.admitQuery(request.Context())
		if !admitted {
			responseWriter.Header().Set("Retry-After", "1")
			writeAPIError(responseWriter, http.StatusServiceUnavailable, "OVERLOADED", "too many requests are running; retry shortly")
			return
		}
		defer release()
		queryContext, cancel := context.WithTimeout(request.Context(), server.requestTimeout)
		defer cancel()
		handle(responseWriter, request.WithContext(queryContext), principal)
	}
}

const (
	defaultConcurrentQueries = 4
	defaultRequestTimeout    = 20 * time.Second
	// queryWait is how long a request waits for a free slot before it is refused.
	// It absorbs a burst without letting a backlog grow.
	queryWait = 250 * time.Millisecond
)

func positiveOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

// admitQuery takes a query slot, waiting briefly for one. The returned func
// frees the slot.
func (server *Server) admitQuery(ctx context.Context) (func(), bool) {
	timer := time.NewTimer(queryWait)
	defer timer.Stop()
	select {
	case server.queryGate <- struct{}{}:
		return func() { <-server.queryGate }, true
	case <-timer.C:
		return nil, false
	case <-ctx.Done():
		return nil, false
	}
}

func (server *Server) availableAPIRoutes() []apiRouteDefinition {
	routes := make([]apiRouteDefinition, 0, len(apiRouteDefinitions))
	for _, route := range apiRouteDefinitions {
		if route.requiresReplay && server.replay == nil {
			continue
		}
		routes = append(routes, route)
	}
	return routes
}

// corsMethods derives the browser-facing method list from the API manifest so a
// newly added mutation route cannot silently be blocked by CORS preflight.
func corsMethods(routes []apiRouteDefinition) string {
	allowed := map[string]bool{
		http.MethodGet:     true,
		http.MethodHead:    true,
		http.MethodOptions: true,
	}
	for _, route := range routes {
		allowed[route.method] = true
	}

	ordered := []string{
		http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodOptions,
	}
	methods := make([]string, 0, len(ordered))
	for _, method := range ordered {
		if allowed[method] {
			methods = append(methods, method)
		}
	}
	return strings.Join(methods, ", ")
}
