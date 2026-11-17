package openapi

import (
	"fmt"
	"strings"
)

// ScopeDoc explains one authorization scope a route can require.
type ScopeDoc struct {
	Name        string
	Description string
	GrantedTo   string
}

// Scopes are the authorization scopes of the API, from least to most privileged.
// "public" routes need no token; every other scope is checked by the router before the handler runs.
var Scopes = []ScopeDoc{
	{"public", "No credentials. Sign-in routes have their own concurrency budget.", "anyone"},
	{"session", "Any signed-in account, without a project.", "every signed-in account"},
	{"read", "Read telemetry, incidents and project details in one project.", "project roles user and owner"},
	{"write", "Change incident status and leave a project.", "project roles user and owner"},
	{"replay", "Replay a captured HTTP request of an incident.", "project role owner"},
	{"admin", "Manage the project: environments, members, ingest tokens, webhooks and audit.", "project role owner"},
	{"platform", "Manage accounts across the installation.", "super administrators"},
}

const bearerScheme = "BearerAuth"

func scopeDoc(name string) (ScopeDoc, bool) {
	for _, scope := range Scopes {
		if scope.Name == name {
			return scope, true
		}
	}
	return ScopeDoc{}, false
}

func securitySchemes() map[string]any {
	var table strings.Builder
	table.WriteString("Access token (JWT, HS256) from /api/v1/auth/login, /register or /refresh; it expires after 15 minutes by default. ")
	table.WriteString("Each operation lists the scope it requires as the role name of this requirement and in x-required-api-scope. ")
	table.WriteString("A super administrator holds every project scope in every project.\n\n| Scope | Granted to | Allows |\n| --- | --- | --- |\n")
	for _, scope := range Scopes {
		if scope.Name == "public" {
			continue
		}
		fmt.Fprintf(&table, "| `%s` | %s | %s |\n", scope.Name, scope.GrantedTo, scope.Description)
	}
	return map[string]any{bearerScheme: map[string]any{
		"type":         "http",
		"scheme":       "bearer",
		"bearerFormat": "JWT",
		"description":  table.String(),
	}}
}

// scopesExtension publishes the scope catalogue for tools that read vendor extensions.
func scopesExtension() map[string]any {
	extension := make(map[string]any, len(Scopes))
	for _, scope := range Scopes {
		extension[scope.Name] = map[string]any{"description": scope.Description, "granted_to": scope.GrantedTo}
	}
	return extension
}

// OpenAPISecurity lists the security requirement of a route. OpenAPI 3.1 allows
// role names in a bearer requirement, so the route's scope is named there.
func OpenAPISecurity(route Route) []map[string][]string {
	if route.Public {
		return []map[string][]string{}
	}
	scopes := []string{}
	if route.Scope != "" {
		scopes = append(scopes, route.Scope)
	}
	return []map[string][]string{{bearerScheme: scopes}}
}

// scopeSentence explains a route's authorization in its description.
func scopeSentence(route Route) string {
	if route.Public {
		return "No authentication required."
	}
	scope, ok := scopeDoc(route.Scope)
	if !ok {
		return fmt.Sprintf("Requires scope `%s`.", route.Scope)
	}
	return fmt.Sprintf("Requires scope `%s` (%s).", scope.Name, scope.GrantedTo)
}
