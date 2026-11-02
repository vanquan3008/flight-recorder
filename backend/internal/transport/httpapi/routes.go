package httpapi

import (
	"context"
	"errors"
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
	// public routes need no credentials; they authenticate the request themselves.
	public bool
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
	{path: "/api/v1/auth/status", method: http.MethodGet, scope: "public", public: true, tags: []string{"auth"}, operationID: "getAuthStatus", handler: func(s *Server) apiHandler { return s.authStatus }},
	{path: "/api/v1/auth/register", method: http.MethodPost, scope: "public", public: true, tags: []string{"auth"}, operationID: "register", successStatus: http.StatusCreated, requestBody: true, handler: func(s *Server) apiHandler { return s.authRegister }},
	{path: "/api/v1/auth/refresh", method: http.MethodPost, scope: "public", public: true, tags: []string{"auth"}, operationID: "refreshSession", successStatus: http.StatusOK, handler: func(s *Server) apiHandler { return s.authRefresh }},
	{path: "/api/v1/auth/login", method: http.MethodPost, scope: "public", public: true, tags: []string{"auth"}, operationID: "login", successStatus: http.StatusOK, requestBody: true, handler: func(s *Server) apiHandler { return s.authLogin }},
	{path: "/api/v1/auth/logout", method: http.MethodPost, scope: "public", public: true, tags: []string{"auth"}, operationID: "logout", successStatus: http.StatusNoContent, handler: func(s *Server) apiHandler { return s.authLogout }},
	{path: "/api/v1/auth/me", method: http.MethodGet, scope: auth.ScopeSession, tags: []string{"auth"}, operationID: "getCurrentUser", handler: func(s *Server) apiHandler { return s.authMe }},
	{path: "/api/v1/auth/password", method: http.MethodPost, scope: auth.ScopeSession, tags: []string{"auth"}, operationID: "changePassword", successStatus: http.StatusOK, requestBody: true, handler: func(s *Server) apiHandler { return s.authChangePassword }},
	{path: "/api/v1/projects", method: http.MethodGet, scope: auth.ScopeSession, tags: []string{"projects"}, operationID: "listProjects", handler: func(s *Server) apiHandler { return s.listProjects }},
	{path: "/api/v1/projects", method: http.MethodPost, scope: auth.ScopeSession, tags: []string{"projects"}, operationID: "createProject", requestBody: true, handler: func(s *Server) apiHandler { return s.createProject }},
	{path: "/api/v1/projects/{projectID}", method: http.MethodGet, scope: "read", tags: []string{"projects"}, operationID: "getProject", handler: func(s *Server) apiHandler { return s.getProject }},
	{path: "/api/v1/projects/{projectID}", method: http.MethodPatch, scope: "admin", tags: []string{"projects"}, operationID: "updateProject", requestBody: true, successStatus: http.StatusOK, handler: func(s *Server) apiHandler { return s.renameProject }},
	{path: "/api/v1/projects/{projectID}/environments", method: http.MethodPost, scope: "admin", tags: []string{"projects"}, operationID: "createEnvironment", requestBody: true, handler: func(s *Server) apiHandler { return s.createEnvironment }},
	{path: "/api/v1/projects/{projectID}/members", method: http.MethodGet, scope: "admin", tags: []string{"projects", "members"}, operationID: "listProjectMembers", handler: func(s *Server) apiHandler { return s.listMembers }},
	{path: "/api/v1/projects/{projectID}/members", method: http.MethodPost, scope: "admin", tags: []string{"projects", "members"}, operationID: "addProjectMember", requestBody: true, handler: func(s *Server) apiHandler { return s.addMember }},
	{path: "/api/v1/projects/{projectID}/members/{userID}", method: http.MethodPatch, scope: "admin", tags: []string{"projects", "members"}, operationID: "changeProjectMemberRole", requestBody: true, successStatus: http.StatusOK, handler: func(s *Server) apiHandler { return s.changeMemberRole }},
	{path: "/api/v1/projects/{projectID}/members/{userID}", method: http.MethodDelete, scope: "write", tags: []string{"projects", "members"}, operationID: "removeProjectMember", handler: func(s *Server) apiHandler { return s.removeMember }},
	{path: "/api/v1/projects/{projectID}/ingest-tokens", method: http.MethodGet, scope: "admin", tags: []string{"projects", "ingest-tokens"}, operationID: "listIngestTokens", handler: func(s *Server) apiHandler { return s.listIngestTokens }},
	{path: "/api/v1/projects/{projectID}/ingest-tokens", method: http.MethodPost, scope: "admin", tags: []string{"projects", "ingest-tokens"}, operationID: "createIngestToken", requestBody: true, handler: func(s *Server) apiHandler { return s.createIngestToken }},
	{path: "/api/v1/projects/{projectID}/ingest-tokens/{tokenID}", method: http.MethodDelete, scope: "admin", tags: []string{"projects", "ingest-tokens"}, operationID: "revokeIngestToken", handler: func(s *Server) apiHandler { return s.revokeIngestToken }},
	{path: "/api/v1/audit-events", method: http.MethodGet, scope: "admin", tags: []string{"audit"}, operationID: "listAuditEvents", handler: func(s *Server) apiHandler { return s.listAuditEvents }},
	{path: "/api/v1/users", method: http.MethodGet, scope: auth.ScopePlatform, tags: []string{"users"}, operationID: "listUsers", handler: func(s *Server) apiHandler { return s.listUsers }},
	{path: "/api/v1/users", method: http.MethodPost, scope: auth.ScopePlatform, tags: []string{"users"}, operationID: "createUser", requestBody: true, handler: func(s *Server) apiHandler { return s.createUser }},
	{path: "/api/v1/users/{userID}", method: http.MethodPatch, scope: auth.ScopePlatform, tags: []string{"users"}, operationID: "updateUser", requestBody: true, successStatus: http.StatusOK, handler: func(s *Server) apiHandler { return s.updateUser }},
	{path: "/api/v1/users/{userID}/password-reset", method: http.MethodPost, scope: auth.ScopePlatform, tags: []string{"users"}, operationID: "resetUserPassword", successStatus: http.StatusOK, handler: func(s *Server) apiHandler { return s.resetUserPassword }},
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
		if route.public {
			router.HandleFunc(route.method+" "+route.path, server.publicRoute(route.handler(server)))
			continue
		}
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
		if principal.ProjectID != "" {
			environmentID, environmentError := server.resolveEnvironment(request.Context(), principal.ProjectID, strings.TrimSpace(request.Header.Get(environmentHeader)))
			if errors.Is(environmentError, errEnvironmentNotFound) {
				writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_ENVIRONMENT", "unknown environment for this project")
				return
			}
			if environmentError != nil {
				server.writeInternalError(responseWriter, "resolve environment", environmentError)
				return
			}
			principal.EnvironmentID = environmentID
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

// publicRoute serves a route that needs no credentials under the same slot and deadline
// limits as authenticated ones, so sign-in cannot be used to exhaust the pool.
func (server *Server) publicRoute(handle apiHandler) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		release, admitted := server.admitQuery(request.Context())
		if !admitted {
			responseWriter.Header().Set("Retry-After", "1")
			writeAPIError(responseWriter, http.StatusServiceUnavailable, "OVERLOADED", "too many requests are running; retry shortly")
			return
		}
		defer release()
		queryContext, cancel := context.WithTimeout(request.Context(), server.requestTimeout)
		defer cancel()
		handle(responseWriter, request.WithContext(queryContext), auth.Principal{})
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
