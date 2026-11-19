package openapi

import (
	"maps"
	"slices"
	"strings"
)

// MaxTraceSpansPerPage bounds one page of a trace's spans.
const MaxTraceSpansPerPage = 500

const (
	maxPageSize = 100

	// Header and cookie names the API reads.
	HeaderProjectID   = "X-Project-ID"
	HeaderEnvironment = "X-Environment"
	HeaderRefresh     = "X-TracePilot-Refresh"
	CookieRefresh     = "tp_refresh"
)

func queryParameter(name, description string, schema map[string]any) map[string]any {
	parameter := map[string]any{"name": name, "in": "query", "schema": schema}
	if description != "" {
		parameter["description"] = description
	}
	return parameter
}

func stringSchema() map[string]any { return map[string]any{"type": "string"} }

func dateTimeSchema() map[string]any { return map[string]any{"type": "string", "format": "date-time"} }

// parameterComponents are the reusable parameters routes refer to by name.
// Each function returns a fresh map so callers cannot change the shared definition.
var parameterComponents = map[string]func() map[string]any{
	"Limit": func() map[string]any {
		return queryParameter("limit", "Items per page.", map[string]any{"type": "integer", "minimum": 1, "maximum": maxPageSize, "default": 50})
	},
	"Cursor": func() map[string]any {
		return queryParameter("cursor", "Opaque next_cursor from the previous page.", stringSchema())
	},
	"SpanLimit": func() map[string]any {
		return queryParameter("limit", "Spans per page; next_cursor is present while more follow.", map[string]any{"type": "integer", "minimum": 1, "maximum": MaxTraceSpansPerPage, "default": MaxTraceSpansPerPage})
	},
	"From": func() map[string]any {
		return queryParameter("from", "Start of the time window (RFC 3339).", dateTimeSchema())
	},
	"To": func() map[string]any {
		return queryParameter("to", "End of the time window (RFC 3339).", dateTimeSchema())
	},
	"TraceListFrom": func() map[string]any {
		return queryParameter("from", "Start of the window. When omitted without traceId, the list covers the hour before to, the page cursor, or now.", dateTimeSchema())
	},
	"LogListFrom": func() map[string]any {
		return queryParameter("from", "Start of the window. When omitted without traceId, the list covers the hour before to, the page cursor, or now.", dateTimeSchema())
	},
	"Service":   func() map[string]any { return queryParameter("service", "Service name.", stringSchema()) },
	"Operation": func() map[string]any { return queryParameter("operation", "Span operation name.", stringSchema()) },
	"Status":    func() map[string]any { return queryParameter("status", "Status filter.", stringSchema()) },
	"Severity":  func() map[string]any { return queryParameter("severity", "Severity filter.", stringSchema()) },
	"LogSeverity": func() map[string]any {
		return queryParameter("severity", "Severity group: TRACE, DEBUG, INFO, WARN, ERROR or FATAL (case-insensitive; numbered levels and INFORMATION/WARNING are aliases). OTLP severity_number takes precedence over text; unspecified numbers fall back to normalized text. Custom text is matched case-insensitively. Raw exporter text is preserved.", stringSchema())
	},
	"TraceID": func() map[string]any {
		return queryParameter("traceId", "Trace ID (32 hex characters).", stringSchema())
	},
	"SpanID": func() map[string]any { return queryParameter("spanId", "Span ID (16 hex characters).", stringSchema()) },
	"Search": func() map[string]any {
		return queryParameter("search", "Text to find in the log body.", stringSchema())
	},
	"Metric": func() map[string]any { return queryParameter("metric", "Metric name.", stringSchema()) },
	"Endpoint": func() map[string]any {
		return queryParameter("endpoint", "HTTP endpoint (method and route).", stringSchema())
	},
	"MinDuration": func() map[string]any {
		return queryParameter("minDuration", "Shortest span duration, for example 250ms.", map[string]any{"type": "string", "example": "250ms"})
	},
	"MaxDuration": func() map[string]any {
		return queryParameter("maxDuration", "Longest span duration, for example 5s.", map[string]any{"type": "string", "example": "5s"})
	},
	"ProjectHeader": func() map[string]any {
		return map[string]any{"name": HeaderProjectID, "in": "header", "required": true, "description": "Project ID or slug the request reads or changes.", "schema": stringSchema()}
	},
	"EnvironmentHeader": func() map[string]any {
		return map[string]any{"name": HeaderEnvironment, "in": "header", "description": "Environment name; defaults to the project's first environment.", "schema": stringSchema()}
	},
	"RefreshHeader": func() map[string]any {
		return map[string]any{"name": HeaderRefresh, "in": "header", "required": true, "description": "Any non-empty value. Proves the request comes from the console, so the refresh cookie cannot be used cross-site.", "schema": map[string]any{"type": "string", "example": "1"}}
	},
	"RefreshCookie": func() map[string]any {
		return map[string]any{"name": CookieRefresh, "in": "cookie", "description": "HttpOnly refresh token set by sign-in and refresh.", "schema": stringSchema()}
	},
}

// HasParameter reports whether name is a reusable parameter.
func HasParameter(name string) bool {
	_, ok := parameterComponents[name]
	return ok
}

// ParameterNames lists every reusable parameter.
func ParameterNames() []string {
	return slices.Sorted(maps.Keys(parameterComponents))
}

func parametersComponent() map[string]any {
	components := make(map[string]any, len(parameterComponents))
	for name, build := range parameterComponents {
		components[name] = build()
	}
	return components
}

// pathParameters documents each {name} segment of a path. IDs other than trace IDs are UUIDs.
func pathParameters(path string) []map[string]any {
	parameters := []map[string]any{}
	for segment := range strings.SplitSeq(path, "/") {
		if !strings.HasPrefix(segment, "{") || !strings.HasSuffix(segment, "}") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")
		schema := stringSchema()
		if strings.HasSuffix(name, "ID") && name != "traceID" {
			schema["format"] = "uuid"
		}
		parameters = append(parameters, map[string]any{"name": name, "in": "path", "required": true, "schema": schema})
	}
	return parameters
}

// RouteParameters returns a route's path parameters followed by references to its header and query parameters.
// Unknown names are skipped; tests in the router check that every declared name exists.
func RouteParameters(route Route) []map[string]any {
	parameters := pathParameters(route.Path)
	for _, name := range slices.Concat(route.Headers, route.Query) {
		if HasParameter(name) {
			parameters = append(parameters, ref("parameters", name))
		}
	}
	return parameters
}

// OpenAPIParameters returns the path parameters of a route.
//
// Deprecated: query parameters are declared on Route.Query; use RouteParameters.
func OpenAPIParameters(path, operationID string) []map[string]any {
	return RouteParameters(Route{Path: path, OperationID: operationID})
}
