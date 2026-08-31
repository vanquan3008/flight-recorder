package management

import "net/http"

func (server *Server) openAPI(responseWriter http.ResponseWriter, _ *http.Request) {
	writeJSONValue(responseWriter, http.StatusOK, openAPISpec())
}

func openAPISpec() map[string]any {
	paths := make(map[string]map[string]any)
	for _, route := range []struct{ path, method, scope string }{
		{"/api/v1/traces", "get", "read"}, {"/api/v1/traces/{traceID}", "get", "read"}, {"/api/v1/traces/{traceID}/logs", "get", "read"},
		{"/api/v1/logs", "get", "read"}, {"/api/v1/logs/{eventID}", "get", "read"}, {"/api/v1/services", "get", "read"}, {"/api/v1/services/{service}/dependencies", "get", "read"},
		{"/api/v1/incidents", "get", "read"}, {"/api/v1/incidents/{incidentID}", "get", "read"}, {"/api/v1/incidents/{incidentID}/occurrences", "get", "read"}, {"/api/v1/incidents/{incidentID}/events", "get", "read"}, {"/api/v1/incidents/{incidentID}/status-changes", "get", "read"}, {"/api/v1/incidents/{incidentID}/executions", "get", "read"}, {"/api/v1/incidents/{incidentID}/executions/{executionID}", "get", "read"},
		{"/api/v1/incidents/{incidentID}", "patch", "write"}, {"/api/v1/incidents/{incidentID}/replays", "post", "replay"},
		{"/api/v1/api-keys", "get", "admin"}, {"/api/v1/api-keys", "post", "admin"}, {"/api/v1/api-keys/{keyID}/rotate", "post", "admin"}, {"/api/v1/api-keys/{keyID}", "delete", "admin"}, {"/api/v1/api-key-events", "get", "admin"},
	} {
		if paths[route.path] == nil {
			paths[route.path] = make(map[string]any)
		}
		paths[route.path][route.method] = map[string]any{
			"security":             []map[string][]string{{"APIKey": {}}},
			"x-required-api-scope": route.scope,
			"responses":            map[string]any{"200": map[string]string{"description": "Success"}, "400": map[string]string{"$ref": "#/components/responses/InvalidRequest"}, "401": map[string]string{"$ref": "#/components/responses/Unauthorized"}, "403": map[string]string{"$ref": "#/components/responses/Forbidden"}, "404": map[string]string{"$ref": "#/components/responses/NotFound"}},
		}
	}
	return map[string]any{
		"openapi": "3.1.0", "info": map[string]string{"title": "TracePilot API", "version": "v1"}, "paths": paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{"APIKey": map[string]string{"type": "apiKey", "in": "header", "name": apiKeyHeader}},
			"schemas":         map[string]any{"Error": map[string]any{"type": "object", "required": []string{"error"}, "example": map[string]any{"error": map[string]string{"code": "INVALID_REQUEST", "message": "request is invalid"}}}, "Page": map[string]any{"type": "object", "required": []string{"data"}}},
			"responses":       map[string]any{"InvalidRequest": map[string]string{"description": "Invalid request"}, "Unauthorized": map[string]string{"description": "Missing, revoked, or expired API key"}, "Forbidden": map[string]string{"description": "API key lacks required scope"}, "NotFound": map[string]string{"description": "Resource does not exist"}},
		},
	}
}
