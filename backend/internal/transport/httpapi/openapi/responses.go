package openapi

import (
	"net/http"
	"strconv"
	"strings"
)

// errorResponses are the reusable error responses; every error body is the ErrorResponse schema.
var errorResponses = []struct {
	status      int
	name        string
	description string
	retryAfter  bool
}{
	{http.StatusBadRequest, "BadRequest", "The request, a query parameter or the environment is invalid.", false},
	{http.StatusUnauthorized, "Unauthorized", "Missing, invalid or expired credentials. An expired access token returns code TOKEN_EXPIRED; refresh it and retry.", false},
	{http.StatusForbidden, "Forbidden", "Your role does not allow this action, the project is not available to you, or the account must change its temporary password.", false},
	{http.StatusNotFound, "NotFound", "The resource does not exist in this project and environment.", false},
	{http.StatusConflict, "Conflict", "The change conflicts with the current state, for example a duplicate name or a disallowed status transition.", false},
	{http.StatusRequestEntityTooLarge, "PayloadTooLarge", "The request body exceeds the size limit.", false},
	{http.StatusTooManyRequests, "RateLimited", "Too many requests from this client or for this account.", true},
	{http.StatusInternalServerError, "InternalError", "Unexpected server error. The response carries a request_id for the server log.", false},
	{http.StatusServiceUnavailable, "Unavailable", "The server is at its concurrency limit or a dependency is unavailable; retry after the given delay.", true},
	{http.StatusGatewayTimeout, "Timeout", "The query exceeded the request deadline; narrow the time range or filters.", false},
}

// Error schema names avoid "Error", which client generators reserve for the language's own type.
const (
	errorResponseSchema = "ErrorResponse"
	errorDetailSchema   = "ErrorDetail"
)

// addErrorSchemas registers the error envelope every error response uses.
func addErrorSchemas(registry *schemas) {
	registry.add(errorDetailSchema, map[string]any{
		"type":     "object",
		"required": []string{"code", "message", "request_id"},
		"properties": map[string]any{
			"code":       map[string]any{"type": "string", "description": "Stable machine-readable code, for example INVALID_REQUEST or TOKEN_EXPIRED."},
			"message":    map[string]any{"type": "string", "description": "Human-readable explanation."},
			"request_id": map[string]any{"type": "string", "description": "Value of the X-Request-ID response header."},
		},
	})
	registry.add(errorResponseSchema, map[string]any{
		"type":       "object",
		"required":   []string{"error"},
		"properties": map[string]any{"error": ref("schemas", errorDetailSchema)},
		"example":    map[string]any{"error": map[string]any{"code": "INVALID_REQUEST", "message": "request is invalid", "request_id": "4f3c"}},
	})
}

func responsesComponent() map[string]any {
	components := make(map[string]any, len(errorResponses))
	for _, entry := range errorResponses {
		response := map[string]any{
			"description": entry.description,
			"content":     map[string]any{"application/json": map[string]any{"schema": ref("schemas", errorResponseSchema)}},
		}
		if entry.retryAfter {
			response["headers"] = map[string]any{"Retry-After": map[string]any{
				"description": "Seconds to wait before retrying.",
				"schema":      map[string]any{"type": "integer", "minimum": 0},
			}}
		}
		components[entry.name] = response
	}
	return components
}

func errorResponseName(status int) string {
	for _, entry := range errorResponses {
		if entry.status == status {
			return entry.name
		}
	}
	return ""
}

// successStatus is the configured status, or the method's convention.
func successStatus(method string, configured int) int {
	if configured != 0 {
		return configured
	}
	switch method {
	case http.MethodPost:
		return http.StatusCreated
	case http.MethodDelete:
		return http.StatusNoContent
	}
	return http.StatusOK
}

func successDescription(status int) string {
	switch status {
	case http.StatusCreated:
		return "Created"
	case http.StatusNoContent:
		return "No content"
	}
	return "Success"
}

// errorStatuses lists the errors a route can return given how the router serves it.
func errorStatuses(route Route, streaming bool) []int {
	statuses := []int{http.StatusBadRequest, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable}
	statuses = append(statuses, http.StatusUnauthorized)
	if !route.Public {
		statuses = append(statuses, http.StatusForbidden)
		if !streaming {
			statuses = append(statuses, http.StatusGatewayTimeout)
		}
	}
	if strings.Contains(route.Path, "{") {
		statuses = append(statuses, http.StatusNotFound)
	}
	switch route.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		statuses = append(statuses, http.StatusConflict)
	}
	if route.RequestBody || route.Request != nil {
		statuses = append(statuses, http.StatusRequestEntityTooLarge)
	}
	return statuses
}

// routeResponses documents the success body and the route's error responses.
func routeResponses(route Route, registry *schemas) map[string]any {
	status := successStatus(route.Method, route.SuccessStatus)
	success := map[string]any{"description": successDescription(status)}
	switch route.Response.Kind {
	case BodyObject:
		success["content"] = jsonContent(registry.schemaFor(route.Response.Type))
	case BodyPage:
		success["content"] = jsonContent(registry.envelope(route.Response.Type, "Page", true))
	case BodyList:
		success["content"] = jsonContent(registry.envelope(route.Response.Type, "List", false))
	case BodyEventStream:
		success["description"] = "Server-Sent Events stream. Each data frame is one JSON event; a comment line is sent every 15 seconds to keep the connection open."
		success["content"] = map[string]any{"text/event-stream": map[string]any{"schema": map[string]any{"type": "string"}}}
	case BodyUntyped:
		if status != http.StatusNoContent {
			success["content"] = jsonContent(map[string]any{"type": "object"})
		}
	}
	responses := map[string]any{strconv.Itoa(status): success}
	for _, code := range errorStatuses(route, route.Response.Kind == BodyEventStream) {
		responses[strconv.Itoa(code)] = ref("responses", errorResponseName(code))
	}
	return responses
}

func jsonContent(schema map[string]any) map[string]any {
	return map[string]any{"application/json": map[string]any{"schema": schema}}
}

// OpenAPIResponses documents the success status for a method and every reusable error response.
//
// Deprecated: use SpecForRoutes with Route.Response, which also documents the success body.
func OpenAPIResponses(method string, configuredSuccessStatus int) map[string]any {
	status := successStatus(method, configuredSuccessStatus)
	responses := map[string]any{strconv.Itoa(status): map[string]any{"description": successDescription(status)}}
	for _, entry := range errorResponses {
		responses[strconv.Itoa(entry.status)] = ref("responses", entry.name)
	}
	return responses
}
