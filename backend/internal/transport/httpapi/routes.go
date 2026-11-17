package httpapi

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
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
	handler        func(*Server, http.ResponseWriter, *http.Request, auth.Principal)
	requiresReplay bool
	// public routes need no credentials; they authenticate the request themselves.
	public    bool
	streaming bool
	// OpenAPI metadata: reusable query and header parameter names, the request body type and the success body.
	query    []string
	headers  []string
	request  reflect.Type
	response openapi.Body
}

var streamRoutes = []apiRouteDefinition{
	{
		path:          "/api/v1/live/stream",
		method:        http.MethodGet,
		scope:         auth.ScopeRead,
		tags:          []string{"Telemetric Streaming"},
		operationID:   "streamLiveTelemetry",
		successStatus: http.StatusOK,
		handler:       (*Server).streamLiveTelemetry,
		streaming:     true,
	},
}

// Each feature file declares its own routes. The router, OpenAPI document and CORS method list all
// read this combined list, so no surface can miss an endpoint.
var apiRouteDefinitions = slices.Concat(
	traceRoutes, analysisRoutes, incidentRoutes, authRoutes, projectRoutes, userRoutes, webhookRoutes, streamRoutes,
)

func (server *Server) registerAPIRoutes(router *http.ServeMux) {
	for _, route := range server.availableAPIRoutes() {
		handle := func(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
			route.handler(server, responseWriter, request, principal)
		}
		if route.public {
			router.HandleFunc(route.method+" "+route.path, server.publicRoute(handle))
			continue
		}
		if route.streaming {
			router.HandleFunc(route.method+" "+route.path, server.requireScopeStreaming(route.scope, handle))
			continue
		}
		router.HandleFunc(route.method+" "+route.path, server.requireScope(route.scope, handle))
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

// requireScopeStreaming authenticates a long-lived streaming connection against the route's scope
// without holding bounded query concurrency slots or timing out after the standard request deadline.
func (server *Server) requireScopeStreaming(scope string, handle apiHandler) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		principal, ok := server.authenticateScope(responseWriter, request, scope)
		if !ok {
			return
		}
		claims, err := auth.VerifyJWTWithIssuerAudience(server.jwtSecret, bearerToken(request), auth.TokenAccess, server.issuer, server.audience, time.Now())
		if err != nil || !time.Now().Before(claims.ExpiresAt) {
			writeAPIError(responseWriter, http.StatusUnauthorized, "TOKEN_EXPIRED", "authentication required")
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
		ctx, cancel := server.watchStreamAccess(request.Context(), principal, claims)
		defer cancel()
		handle(responseWriter, request.WithContext(ctx), principal)
	}
}

// publicRoute serves a route that needs no credentials under its own bounded slot budget
// so sign-ins cannot exhaust the pool and are never starved by telemetry queries.
func (server *Server) publicRoute(handle apiHandler) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		release, admitted := server.admitAuth(request.Context())
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
	defaultConcurrentQueries     = 4
	defaultAuthConcurrentQueries = 16
	defaultRequestTimeout        = 20 * time.Second
	// queryWait is how long a request waits for a free slot before it is refused.
	// It absorbs a burst without letting a backlog grow.
	queryWait = 500 * time.Millisecond
	authWait  = 500 * time.Millisecond
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
	start := time.Now()
	timer := time.NewTimer(queryWait)
	defer timer.Stop()
	select {
	case server.queryGate <- struct{}{}:
		server.metrics.ObserveGateWait("query", time.Since(start))
		server.metrics.IncGateActive("query")
		return func() {
			<-server.queryGate
			server.metrics.DecGateActive("query")
		}, true
	case <-timer.C:
		server.metrics.ObserveGateWait("query", time.Since(start))
		return nil, false
	case <-ctx.Done():
		server.metrics.ObserveGateWait("query", time.Since(start))
		return nil, false
	}
}

// admitAuth takes an authentication slot, waiting briefly for one.
func (server *Server) admitAuth(ctx context.Context) (func(), bool) {
	start := time.Now()
	timer := time.NewTimer(authWait)
	defer timer.Stop()
	select {
	case server.authGate <- struct{}{}:
		server.metrics.ObserveGateWait("auth", time.Since(start))
		server.metrics.IncGateActive("auth")
		return func() {
			<-server.authGate
			server.metrics.DecGateActive("auth")
		}, true
	case <-timer.C:
		server.metrics.ObserveGateWait("auth", time.Since(start))
		return nil, false
	case <-ctx.Done():
		server.metrics.ObserveGateWait("auth", time.Since(start))
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
