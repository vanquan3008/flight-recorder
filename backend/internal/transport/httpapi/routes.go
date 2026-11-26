package httpapi

import (
	"net/http"
	"slices"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/access"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/analysis"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/incidents"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/ingesttokens"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/logs"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/members"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/projects"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/session"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/stream"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/traces"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/users"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/webhooks"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

// Backends binds each feature to the port it consumes. Optional backends can be left nil.
type Backends struct {
	Access           access.Store
	Session          session.Backend
	Users            users.Backend
	Projects         projects.Backend
	ProjectRetention projects.RetentionBackend
	Members          members.Backend
	IngestTokens     ingesttokens.Backend
	Traces           traces.Backend
	Logs             logs.Backend
	Services         analysis.Backend
	Insights         analysis.InsightsBackend
	Performance      analysis.PerformanceBackend
	Incidents        incidents.Backend
	IncidentAnalysis incidents.AnalysisBackend
	Webhooks         webhooks.Backend
}

// featureRoutes builds every feature and returns their combined routes. The router, the OpenAPI
// document, and the CORS method list all read this list, so no surface can miss an endpoint.
func (server *Server) featureRoutes(configuration config.Config, dependencies Dependencies) []api.Route {
	backends, logger := dependencies.Backends, dependencies.Logger
	webhookStore := backends.Webhooks
	if !configuration.Notifications.Enabled {
		webhookStore = nil
	}
	features := []interface{ Routes() []api.Route }{
		traces.New(backends.Traces, logger),
		logs.New(backends.Logs, logger),
		analysis.New(backends.Services, backends.Insights, backends.Performance, logger),
		incidents.New(backends.Incidents, backends.IncidentAnalysis, dependencies.Replay, configuration.Replay.MaxRequestBytes, logger),
		session.New(session.Config{
			TokenSecret:     []byte(configuration.Auth.JWTSecret),
			TokenIssuer:     configuration.Auth.EffectiveIssuer(),
			TokenAudience:   configuration.Auth.EffectiveAudience(),
			AccessTokenTTL:  configuration.Auth.AccessTokenTTL,
			RefreshTokenTTL: configuration.Auth.RefreshTokenTTL,
			CookieDomain:    configuration.Auth.CookieDomain,
			CookieSameSite:  configuration.Auth.CookieSameSiteMode(),
			Production:      configuration.App.Production,
			TLS:             server.tlsCertificate != "",
			TrustedProxies:  server.trustedProxies,
		}, backends.Session, server.rateLimiter, logger),
		projects.New(backends.Projects, backends.ProjectRetention, server.guard, configuration.Storage.Retention.MaxAge, logger),
		members.New(backends.Members, server.guard, logger),
		ingesttokens.New(backends.IngestTokens, []byte(configuration.Auth.EffectiveIngestJWTSecret()), dependencies.TokenInvalidator, logger),
		users.New(backends.Users, logger),
		webhooks.New(webhookStore, logger),
		stream.New(dependencies.StreamHub),
	}
	var routes []api.Route
	for _, feature := range features {
		routes = append(routes, feature.Routes()...)
	}
	return routes
}

// RegisterAPIRoutes mounts the API routes through their admission, without the shared middleware, so
// integration tests can drive handlers against a real database without tripping rate limits.
func (server *Server) RegisterAPIRoutes(router *http.ServeMux) {
	for _, route := range server.routes {
		router.HandleFunc(route.Method+" "+route.Path, server.guard.Admit(route))
	}
}

func (server *Server) openAPI(responseWriter http.ResponseWriter, _ *http.Request) {
	render.WriteJSON(responseWriter, http.StatusOK, openAPISpec(server.routes))
}

// projectScopes are the scopes the router resolves within one project and environment.
var projectScopes = []string{auth.ScopeRead, auth.ScopeWrite, auth.ScopeReplay, auth.ScopeAdmin}

func openAPISpec(routes []api.Route) map[string]any {
	converted := make([]openapi.Route, len(routes))
	for index, route := range routes {
		converted[index] = openAPIRoute(route)
	}
	return openapi.SpecForRoutes(converted)
}

// openAPIRoute adds what the router itself requires: the project and environment headers of project
// routes and the stream body of SSE routes.
func openAPIRoute(route api.Route) openapi.Route {
	headers := slices.Clone(route.Headers)
	if slices.Contains(projectScopes, route.Scope) {
		if !strings.Contains(route.Path, "{projectID}") {
			headers = append(headers, "ProjectHeader")
		}
		headers = append(headers, "EnvironmentHeader")
	}
	response := route.Response
	if route.Streaming {
		response = openapi.EventStream()
	}
	return openapi.Route{
		Path:          route.Path,
		Method:        route.Method,
		Scope:         route.Scope,
		Tags:          route.Tags,
		OperationID:   route.OperationID,
		SuccessStatus: route.SuccessStatus,
		RequestBody:   route.RequestBody,
		Public:        route.Public,
		Query:         route.Query,
		Headers:       headers,
		Request:       route.Request,
		Response:      response,
	}
}

// corsMethods derives the browser-facing method list from the routes so a newly added mutation route
// cannot silently be blocked by CORS preflight.
func corsMethods(routes []api.Route) string {
	allowed := map[string]bool{http.MethodGet: true, http.MethodHead: true, http.MethodOptions: true}
	for _, route := range routes {
		allowed[route.Method] = true
	}
	ordered := []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions}
	methods := make([]string, 0, len(ordered))
	for _, method := range ordered {
		if allowed[method] {
			methods = append(methods, method)
		}
	}
	return strings.Join(methods, ", ")
}
