package httpapi

import (
	"net/http"
	"strconv"
	"strings"
)

func (server *Server) openAPI(responseWriter http.ResponseWriter, _ *http.Request) {
	writeJSONValue(responseWriter, http.StatusOK, openAPISpecForRoutes(server.availableAPIRoutes()))
}

func openAPISpec() map[string]any {
	return openAPISpecForRoutes(apiRouteDefinitions)
}

func openAPISpecForRoutes(routes []apiRouteDefinition) map[string]any {
	paths := make(map[string]any)
	for _, route := range routes {
		operations, _ := paths[route.path].(map[string]any)
		if operations == nil {
			operations = make(map[string]any)
			paths[route.path] = operations
		}
		operation := map[string]any{
			"operationId":          route.operationID,
			"summary":              routeSummary(route.operationID),
			"tags":                 route.tags,
			"security":             []map[string][]string{{"APIKey": {}}},
			"parameters":           openAPIParameters(route.path, route.operationID),
			"responses":            openAPIResponses(route.method, route.successStatus),
			"x-required-api-scope": route.scope,
		}
		if route.requestBody {
			operation["requestBody"] = map[string]any{
				"required": true,
				"content": map[string]any{"application/json": map[string]any{
					"schema":  map[string]any{"type": "object"},
					"example": map[string]any{},
				}},
			}
		}
		operations[strings.ToLower(route.method)] = operation
	}
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "TracePilot API",
			"version":     "v1",
			"description": "Versioned management API. Additive changes are allowed in v1; breaking changes require /api/v2. Deprecated operations remain for one minor release and emit a Deprecation header.",
		},
		"servers": []map[string]string{{"url": "/"}},
		"paths":   paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{"APIKey": map[string]string{"type": "apiKey", "in": "header", "name": apiKeyHeader}},
			"schemas": map[string]any{
				"Error": map[string]any{
					"type":     "object",
					"required": []string{"error"},
					"properties": map[string]any{"error": map[string]any{
						"type":     "object",
						"required": []string{"code", "message", "request_id"},
						"properties": map[string]any{
							"code":       map[string]string{"type": "string"},
							"message":    map[string]string{"type": "string"},
							"request_id": map[string]string{"type": "string"},
						},
					}},
					"example": map[string]any{"error": map[string]string{"code": "INVALID_REQUEST", "message": "request is invalid", "request_id": "4f3c"}},
				},
				"Page": map[string]any{"type": "object", "required": []string{"data"}, "properties": map[string]any{"data": map[string]any{"type": "array", "items": map[string]any{}}, "next_cursor": map[string]string{"type": "string"}}},
			},
			"responses": map[string]any{
				"InvalidRequest": map[string]any{"description": "Invalid request", "content": map[string]any{"application/json": map[string]any{"schema": map[string]string{"$ref": "#/components/schemas/Error"}}}},
				"Unauthorized":   map[string]any{"description": "Missing, revoked, or expired API key", "content": map[string]any{"application/json": map[string]any{"schema": map[string]string{"$ref": "#/components/schemas/Error"}}}},
				"Forbidden":      map[string]any{"description": "API key lacks required scope", "content": map[string]any{"application/json": map[string]any{"schema": map[string]string{"$ref": "#/components/schemas/Error"}}}},
				"NotFound":       map[string]any{"description": "Resource does not exist", "content": map[string]any{"application/json": map[string]any{"schema": map[string]string{"$ref": "#/components/schemas/Error"}}}},
				"RateLimited":    map[string]any{"description": "Rate limit exceeded", "headers": map[string]any{"Retry-After": map[string]any{"schema": map[string]string{"type": "integer"}}}, "content": map[string]any{"application/json": map[string]any{"schema": map[string]string{"$ref": "#/components/schemas/Error"}}}},
			},
		},
	}
}

func openAPIResponses(method string, configuredSuccessStatus int) map[string]any {
	successCode, successDescription := "200", "Success"
	if configuredSuccessStatus != 0 {
		successCode = strconv.Itoa(configuredSuccessStatus)
		if configuredSuccessStatus == http.StatusCreated {
			successDescription = "Created"
		} else if configuredSuccessStatus == http.StatusNoContent {
			successDescription = "No content"
		}
	} else {
		switch method {
		case http.MethodPost:
			successCode, successDescription = "201", "Created"
		case http.MethodDelete:
			successCode, successDescription = "204", "No content"
		}
	}
	responses := map[string]any{
		successCode: map[string]any{"description": successDescription},
		"400":       map[string]string{"$ref": "#/components/responses/InvalidRequest"},
		"401":       map[string]string{"$ref": "#/components/responses/Unauthorized"},
		"403":       map[string]string{"$ref": "#/components/responses/Forbidden"},
		"404":       map[string]string{"$ref": "#/components/responses/NotFound"},
		"409":       map[string]any{"description": "Conflict"},
		"413":       map[string]any{"description": "Request body too large"},
		"429":       map[string]string{"$ref": "#/components/responses/RateLimited"},
		"500":       map[string]any{"description": "Internal server error"},
	}
	return responses
}

func openAPIParameters(path, operationID string) []map[string]any {
	parameters := make([]map[string]any, 0, 8)
	for _, segment := range strings.Split(path, "/") {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			name := strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")
			schema := map[string]any{"type": "string"}
			if strings.HasSuffix(name, "ID") && name != "traceID" {
				schema["format"] = "uuid"
			}
			parameters = append(parameters, map[string]any{"name": name, "in": "path", "required": true, "schema": schema})
		}
	}
	return append(parameters, openAPIQueryParameters(operationID)...)
}

// openAPIQueryParameters mirrors each handler's accepted query string. Keeping
// this explicit is less clever than inferring from a name, but prevents the
// contract from advertising filters that the endpoint silently ignores.
func openAPIQueryParameters(operationID string) []map[string]any {
	switch operationID {
	case "listTraces":
		return combineOpenAPIParameters(
			openAPIPageParameters(),
			openAPITimeParameters(),
			openAPIStringParameters("service", "operation", "status", "traceId"),
			openAPIDurationParameters(),
		)
	case "listLogs":
		return combineOpenAPIParameters(
			openAPIPageParameters(),
			openAPITimeParameters(),
			openAPIStringParameters("service", "severity", "traceId", "spanId", "search"),
		)
	case "listTraceLogs":
		return combineOpenAPIParameters(
			openAPIPageParameters(),
			openAPITimeParameters(),
			openAPIStringParameters("service", "severity", "spanId", "search"),
		)
	case "listTopology", "getTelemetryQuality":
		return combineOpenAPIParameters(
			openAPITimeParameters(),
			openAPIStringParameters("service"),
		)
	case "listMetricPoints":
		return combineOpenAPIParameters(openAPIPageParameters()[:1], openAPITimeParameters(), openAPIStringParameters("service", "metric"))
	case "listIncidents":
		return combineOpenAPIParameters(
			openAPIPageParameters(),
			openAPITimeParameters(),
			openAPIStringParameters("service", "severity", "status"),
		)
	case "listServices":
		return openAPITimeParameters()
	case "getRequestPerformance":
		return combineOpenAPIParameters(openAPIPageParameters()[:1], openAPITimeParameters(), openAPIStringParameters("service", "endpoint"))
	case "listIncidentOccurrences", "listIncidentEvents", "listIncidentStatusChanges", "listReplayExecutions", "listAPIKeys", "listAPIKeyEvents", "listWebhooks", "listWebhookDeliveries":
		return openAPIPageParameters()
	default:
		return nil
	}
}

func combineOpenAPIParameters(groups ...[]map[string]any) []map[string]any {
	parameters := make([]map[string]any, 0, 12)
	for _, group := range groups {
		parameters = append(parameters, group...)
	}
	return parameters
}

func openAPIPageParameters() []map[string]any {
	return []map[string]any{openAPILimitParameter(), openAPIStringParameter("cursor")}
}

func openAPILimitParameter() map[string]any {
	return map[string]any{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}}
}

func openAPITimeParameters() []map[string]any {
	return []map[string]any{
		{"name": "from", "in": "query", "schema": map[string]string{"type": "string", "format": "date-time"}},
		{"name": "to", "in": "query", "schema": map[string]string{"type": "string", "format": "date-time"}},
	}
}

func openAPIDurationParameters() []map[string]any {
	return []map[string]any{
		{"name": "minDuration", "in": "query", "schema": map[string]string{"type": "string", "example": "250ms"}},
		{"name": "maxDuration", "in": "query", "schema": map[string]string{"type": "string", "example": "5s"}},
	}
}

func openAPIStringParameters(names ...string) []map[string]any {
	parameters := make([]map[string]any, 0, len(names))
	for _, name := range names {
		parameters = append(parameters, openAPIStringParameter(name))
	}
	return parameters
}

func openAPIStringParameter(name string) map[string]any {
	return map[string]any{"name": name, "in": "query", "schema": map[string]string{"type": "string"}}
}

func routeSummary(operationID string) string {
	if operationID == "" {
		return "TracePilot management operation"
	}
	return "TracePilot " + operationID
}
