package openapi

import (
	"slices"
	"strings"
	"unicode"
)

const apiDescription = "Versioned management API. Additive changes are allowed in v1; breaking changes require /api/v2. " +
	"Deprecated operations remain for one minor release and emit a Deprecation header.\n\n" +
	"Project routes choose the project with the X-Project-ID header (ID or slug) unless the path names it, " +
	"and the environment with X-Environment. Lists use cursor pagination: pass next_cursor back as cursor."

// SpecForRoutes renders an OpenAPI 3.1.0 document for the given routes.
func SpecForRoutes(routes []Route) map[string]any {
	registry := newSchemas()
	addErrorSchemas(registry)
	paths := make(map[string]any)
	tags := []string{}
	for _, route := range routes {
		operations, _ := paths[route.Path].(map[string]any)
		if operations == nil {
			operations = make(map[string]any)
			paths[route.Path] = operations
		}
		operations[strings.ToLower(route.Method)] = operation(route, registry)
		for _, tag := range route.Tags {
			if !slices.Contains(tags, tag) {
				tags = append(tags, tag)
			}
		}
	}
	slices.Sort(tags)
	tagList := make([]map[string]string, 0, len(tags))
	for _, tag := range tags {
		tagList = append(tagList, map[string]string{"name": tag})
	}
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "TracePilot API",
			"version":     "v1",
			"description": apiDescription,
		},
		"servers":             []map[string]string{{"url": "/"}},
		"tags":                tagList,
		"paths":               paths,
		"x-tracepilot-scopes": scopesExtension(),
		"components": map[string]any{
			"securitySchemes": securitySchemes(),
			"schemas":         registry.components,
			"parameters":      parametersComponent(),
			"responses":       responsesComponent(),
		},
	}
}

func operation(route Route, registry *schemas) map[string]any {
	summary := route.Summary
	if summary == "" {
		summary = RouteSummary(route.OperationID)
	}
	description := scopeSentence(route)
	if route.Description != "" {
		description = route.Description + "\n\n" + description
	}
	operation := map[string]any{
		"operationId":          route.OperationID,
		"summary":              summary,
		"description":          description,
		"tags":                 route.Tags,
		"security":             OpenAPISecurity(route),
		"parameters":           RouteParameters(route),
		"responses":            routeResponses(route, registry),
		"x-required-api-scope": route.Scope,
	}
	switch {
	case route.Request != nil:
		operation["requestBody"] = map[string]any{"required": true, "content": jsonContent(registry.schemaFor(route.Request))}
	case route.RequestBody:
		operation["requestBody"] = map[string]any{"required": true, "content": jsonContent(map[string]any{"type": "object"})}
	}
	return operation
}

// RouteSummary turns an operationID such as listTraceLogs into "List trace logs".
func RouteSummary(operationID string) string {
	if operationID == "" {
		return "TracePilot management operation"
	}
	var words strings.Builder
	runes := []rune(operationID)
	for index, r := range runes {
		if index > 0 && unicode.IsUpper(r) {
			nextLower := index+1 < len(runes) && unicode.IsLower(runes[index+1])
			if !unicode.IsUpper(runes[index-1]) || nextLower {
				words.WriteRune(' ')
			}
		}
		if index == 0 {
			words.WriteRune(unicode.ToUpper(r))
			continue
		}
		if unicode.IsUpper(r) && (index+1 >= len(runes) || unicode.IsLower(runes[index+1])) {
			words.WriteRune(unicode.ToLower(r))
			continue
		}
		words.WriteRune(r)
	}
	return words.String()
}
