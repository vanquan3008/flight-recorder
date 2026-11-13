package httpapi

import (
	"net/http"

	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
)

func (server *Server) openAPI(responseWriter http.ResponseWriter, _ *http.Request) {
	writeJSONValue(responseWriter, http.StatusOK, openAPISpecForRoutes(server.availableAPIRoutes()))
}

func openAPISpecForRoutes(routes []apiRouteDefinition) map[string]any {
	converted := make([]openapi.Route, len(routes))
	for i, r := range routes {
		converted[i] = openapi.Route{
			Path:          r.path,
			Method:        r.method,
			Scope:         r.scope,
			Tags:          r.tags,
			OperationID:   r.operationID,
			SuccessStatus: r.successStatus,
			RequestBody:   r.requestBody,
			Public:        r.public,
		}
	}
	return openapi.SpecForRoutes(converted)
}
