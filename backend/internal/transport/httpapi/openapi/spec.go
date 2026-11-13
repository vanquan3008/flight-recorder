package openapi

import (
	"net/http"
	"strconv"
	"strings"
)

const MaxTraceSpansPerPage = 500

// Route defines the metadata needed to document an API endpoint in OpenAPI 3.1.
type Route struct {
	Path          string
	Method        string
	Scope         string
	Tags          []string
	OperationID   string
	SuccessStatus int
	RequestBody   bool
	Public        bool
}

// SpecForRoutes generates an OpenAPI 3.1.0 specification for the given list of routes.
func SpecForRoutes(routes []Route) map[string]any {
	paths := make(map[string]any)
	for _, route := range routes {
		operations, _ := paths[route.Path].(map[string]any)
		if operations == nil {
			operations = make(map[string]any)
			paths[route.Path] = operations
		}
		operation := map[string]any{
			"operationId":          route.OperationID,
			"summary":              RouteSummary(route.OperationID),
			"tags":                 route.Tags,
			"security":             OpenAPISecurity(route),
			"parameters":           OpenAPIParameters(route.Path, route.OperationID),
			"responses":            OpenAPIResponses(route.Method, route.SuccessStatus),
			"x-required-api-scope": route.Scope,
		}
		if route.RequestBody {
			operation["requestBody"] = map[string]any{
				"required": true,
				"content": map[string]any{"application/json": map[string]any{
					"schema":  map[string]any{"type": "object"},
					"example": map[string]any{},
				}},
			}
		}
		operations[strings.ToLower(route.Method)] = operation
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
			"securitySchemes": map[string]any{
				"BearerAuth": map[string]string{"type": "http", "scheme": "bearer", "bearerFormat": "JWT", "description": "Access token from /api/v1/auth/login or /refresh (15 minutes). Project routes also send X-Project-ID."},
			},
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
				"Unauthorized":   map[string]any{"description": "Missing, invalid, or expired access token", "content": map[string]any{"application/json": map[string]any{"schema": map[string]string{"$ref": "#/components/schemas/Error"}}}},
				"Forbidden":      map[string]any{"description": "Your role in this project does not allow this action", "content": map[string]any{"application/json": map[string]any{"schema": map[string]string{"$ref": "#/components/schemas/Error"}}}},
				"NotFound":       map[string]any{"description": "Resource does not exist", "content": map[string]any{"application/json": map[string]any{"schema": map[string]string{"$ref": "#/components/schemas/Error"}}}},
				"RateLimited":    map[string]any{"description": "Rate limit exceeded", "headers": map[string]any{"Retry-After": map[string]any{"schema": map[string]string{"type": "integer"}}}, "content": map[string]any{"application/json": map[string]any{"schema": map[string]string{"$ref": "#/components/schemas/Error"}}}},
			},
		},
	}
}

// OpenAPISecurity lists authentication requirements for a route.
func OpenAPISecurity(route Route) []map[string][]string {
	if route.Public {
		return []map[string][]string{}
	}
	return []map[string][]string{{"BearerAuth": {}}}
}

// OpenAPIResponses defines standard HTTP responses for a method.
func OpenAPIResponses(method string, configuredSuccessStatus int) map[string]any {
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
	return map[string]any{
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
}

// OpenAPIParameters extracts path and query parameters for a route.
func OpenAPIParameters(path, operationID string) []map[string]any {
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
	case "getTrace":
		return []map[string]any{
			{"name": "limit", "in": "query", "description": "Spans per page; next_cursor is present while more follow.", "schema": map[string]any{"type": "integer", "minimum": 1, "maximum": MaxTraceSpansPerPage}},
			openAPIStringParameter("cursor"),
		}
	case "getRequestPerformance":
		return combineOpenAPIParameters(openAPIPageParameters()[:1], openAPITimeParameters(), openAPIStringParameters("service", "endpoint"))
	case "listIncidentOccurrences", "listIncidentEvents", "listIncidentStatusChanges", "listReplayExecutions", "listIngestTokens", "listWebhooks", "listWebhookDeliveries", "listAuditEvents", "listUsers":
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

// RouteSummary gives a human-readable title to an operationID.
func RouteSummary(operationID string) string {
	if operationID == "" {
		return "TracePilot management operation"
	}
	return "TracePilot " + operationID
}
