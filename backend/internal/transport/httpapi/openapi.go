package httpapi

import (
	"net/http"
	"slices"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
)

func (server *Server) openAPI(responseWriter http.ResponseWriter, _ *http.Request) {
	writeJSONValue(responseWriter, http.StatusOK, openAPISpecForRoutes(server.availableAPIRoutes()))
}

// projectScopes are the scopes the router resolves within one project and environment.
var projectScopes = []string{auth.ScopeRead, auth.ScopeWrite, auth.ScopeReplay, auth.ScopeAdmin}

func openAPISpecForRoutes(routes []apiRouteDefinition) map[string]any {
	converted := make([]openapi.Route, len(routes))
	for i, r := range routes {
		converted[i] = openAPIRoute(r)
	}
	return openapi.SpecForRoutes(converted)
}

// openAPIRoute adds what the router itself requires: the project and
// environment headers of project routes and the stream body of SSE routes.
func openAPIRoute(r apiRouteDefinition) openapi.Route {
	headers := slices.Clone(r.headers)
	if slices.Contains(projectScopes, r.scope) {
		if !strings.Contains(r.path, "{projectID}") {
			headers = append(headers, "ProjectHeader")
		}
		headers = append(headers, "EnvironmentHeader")
	}
	response := r.response
	if r.streaming {
		response = openapi.EventStream()
	}
	return openapi.Route{
		Path:          r.path,
		Method:        r.method,
		Scope:         r.scope,
		Tags:          r.tags,
		OperationID:   r.operationID,
		SuccessStatus: r.successStatus,
		RequestBody:   r.requestBody,
		Public:        r.public,
		Query:         r.query,
		Headers:       headers,
		Request:       r.request,
		Response:      response,
	}
}
