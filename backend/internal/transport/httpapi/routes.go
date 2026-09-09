package httpapi

import (
	"net/http"
	"strings"
)

type apiRouteDefinition struct {
	path           string
	method         string
	scope          string
	tags           []string
	operationID    string
	successStatus  int
	requestBody    bool
	handler        func(*Server) http.HandlerFunc
	requiresReplay bool
}

// Keep the management router and OpenAPI contract sourced from one manifest.
// This prevents adding an endpoint to one surface while silently omitting it
// from the other.
var apiRouteDefinitions = []apiRouteDefinition{
	{path: "/api/v1/traces", method: http.MethodGet, scope: "read", tags: []string{"traces"}, operationID: "listTraces", handler: func(s *Server) http.HandlerFunc { return s.listTraces }},
	{path: "/api/v1/traces/{traceID}", method: http.MethodGet, scope: "read", tags: []string{"traces"}, operationID: "getTrace", handler: func(s *Server) http.HandlerFunc { return s.getTrace }},
	{path: "/api/v1/traces/{traceID}/logs", method: http.MethodGet, scope: "read", tags: []string{"traces", "logs"}, operationID: "listTraceLogs", handler: func(s *Server) http.HandlerFunc { return s.listTraceLogs }},
	{path: "/api/v1/logs", method: http.MethodGet, scope: "read", tags: []string{"logs"}, operationID: "listLogs", handler: func(s *Server) http.HandlerFunc { return s.listLogs }},
	{path: "/api/v1/logs/{eventID}", method: http.MethodGet, scope: "read", tags: []string{"logs"}, operationID: "getLog", handler: func(s *Server) http.HandlerFunc { return s.getLog }},
	{path: "/api/v1/security/events", method: http.MethodGet, scope: "read", tags: []string{"security"}, operationID: "listSecurityEvents", handler: func(s *Server) http.HandlerFunc { return s.listSecurityEvents }},
	{path: "/api/v1/security/events/{eventID}", method: http.MethodGet, scope: "read", tags: []string{"security"}, operationID: "getSecurityEvent", handler: func(s *Server) http.HandlerFunc { return s.getSecurityEvent }},
	{path: "/api/v1/security/sources", method: http.MethodGet, scope: "read", tags: []string{"security"}, operationID: "listSecuritySources", handler: func(s *Server) http.HandlerFunc { return s.listSecuritySources }},
	{path: "/api/v1/security/detections", method: http.MethodGet, scope: "read", tags: []string{"security"}, operationID: "listSecurityDetections", handler: func(s *Server) http.HandlerFunc { return s.listSecurityDetections }},
	{path: "/api/v1/security/rules", method: http.MethodGet, scope: "read", tags: []string{"security-rules"}, operationID: "listSecurityRules", handler: func(s *Server) http.HandlerFunc { return s.listSecurityRules }},
	{path: "/api/v1/security/rules/{ruleID}/history", method: http.MethodGet, scope: "read", tags: []string{"security-rules"}, operationID: "listSecurityRuleAudits", handler: func(s *Server) http.HandlerFunc { return s.listSecurityRuleAudits }},
	{path: "/api/v1/security/rules", method: http.MethodPost, scope: "write", tags: []string{"security-rules"}, operationID: "createSecurityRule", requestBody: true, handler: func(s *Server) http.HandlerFunc { return s.createSecurityRule }},
	{path: "/api/v1/security/rules/{ruleID}", method: http.MethodPut, scope: "write", tags: []string{"security-rules"}, operationID: "updateSecurityRule", requestBody: true, handler: func(s *Server) http.HandlerFunc { return s.updateSecurityRule }},
	{path: "/api/v1/security/rules/{ruleID}", method: http.MethodDelete, scope: "write", tags: []string{"security-rules"}, operationID: "deleteSecurityRule", handler: func(s *Server) http.HandlerFunc { return s.deleteSecurityRule }},
	{path: "/api/v1/services", method: http.MethodGet, scope: "read", tags: []string{"services"}, operationID: "listServices", handler: func(s *Server) http.HandlerFunc { return s.listServices }},
	{path: "/api/v1/services/{service}/dependencies", method: http.MethodGet, scope: "read", tags: []string{"services"}, operationID: "listServiceDependencies", handler: func(s *Server) http.HandlerFunc { return s.listServiceDependencies }},
	{path: "/api/v1/incidents", method: http.MethodGet, scope: "read", tags: []string{"incidents"}, operationID: "listIncidents", handler: func(s *Server) http.HandlerFunc { return s.listIncidents }},
	{path: "/api/v1/incidents/{incidentID}", method: http.MethodGet, scope: "read", tags: []string{"incidents"}, operationID: "getIncident", handler: func(s *Server) http.HandlerFunc { return s.getIncident }},
	{path: "/api/v1/incidents/{incidentID}/occurrences", method: http.MethodGet, scope: "read", tags: []string{"incidents"}, operationID: "listIncidentOccurrences", handler: func(s *Server) http.HandlerFunc { return s.listIncidentOccurrences }},
	{path: "/api/v1/incidents/{incidentID}/events", method: http.MethodGet, scope: "read", tags: []string{"incidents"}, operationID: "listIncidentEvents", handler: func(s *Server) http.HandlerFunc { return s.listIncidentEvents }},
	{path: "/api/v1/incidents/{incidentID}/status-changes", method: http.MethodGet, scope: "read", tags: []string{"incidents"}, operationID: "listIncidentStatusChanges", handler: func(s *Server) http.HandlerFunc { return s.listIncidentStatusChanges }},
	{path: "/api/v1/incidents/{incidentID}/executions", method: http.MethodGet, scope: "read", tags: []string{"replay"}, operationID: "listReplayExecutions", handler: func(s *Server) http.HandlerFunc { return s.listAllowedRequestExecutions }},
	{path: "/api/v1/incidents/{incidentID}/executions/{executionID}", method: http.MethodGet, scope: "read", tags: []string{"replay"}, operationID: "getReplayExecution", handler: func(s *Server) http.HandlerFunc { return s.getAllowedRequestExecution }},
	{path: "/api/v1/incidents/{incidentID}", method: http.MethodPatch, scope: "write", tags: []string{"incidents"}, operationID: "updateIncident", requestBody: true, handler: func(s *Server) http.HandlerFunc { return s.updateIncident }},
	{path: "/api/v1/incidents/{incidentID}/replays", method: http.MethodPost, scope: "replay", tags: []string{"replay"}, operationID: "replayIncident", requestBody: true, requiresReplay: true, handler: func(s *Server) http.HandlerFunc { return s.replayIncident }},
	{path: "/api/v1/api-keys", method: http.MethodGet, scope: "admin", tags: []string{"api-keys"}, operationID: "listAPIKeys", handler: func(s *Server) http.HandlerFunc { return s.listAPIKeys }},
	{path: "/api/v1/api-key-events", method: http.MethodGet, scope: "admin", tags: []string{"api-keys"}, operationID: "listAPIKeyEvents", handler: func(s *Server) http.HandlerFunc { return s.listAPIKeyEvents }},
	{path: "/api/v1/api-keys", method: http.MethodPost, scope: "admin", tags: []string{"api-keys"}, operationID: "createAPIKey", requestBody: true, handler: func(s *Server) http.HandlerFunc { return s.createAPIKey }},
	{path: "/api/v1/api-keys/{keyID}/rotate", method: http.MethodPost, scope: "admin", tags: []string{"api-keys"}, operationID: "rotateAPIKey", successStatus: http.StatusOK, handler: func(s *Server) http.HandlerFunc { return s.rotateAPIKey }},
	{path: "/api/v1/api-keys/{keyID}", method: http.MethodDelete, scope: "admin", tags: []string{"api-keys"}, operationID: "revokeAPIKey", handler: func(s *Server) http.HandlerFunc { return s.revokeAPIKey }},
	{path: "/api/v1/webhooks", method: http.MethodGet, scope: "admin", tags: []string{"webhooks"}, operationID: "listWebhooks", handler: func(s *Server) http.HandlerFunc { return s.listWebhooks }},
	{path: "/api/v1/webhooks", method: http.MethodPost, scope: "admin", tags: []string{"webhooks"}, operationID: "createWebhook", requestBody: true, handler: func(s *Server) http.HandlerFunc { return s.createWebhook }},
	{path: "/api/v1/webhooks/{webhookID}", method: http.MethodGet, scope: "admin", tags: []string{"webhooks"}, operationID: "getWebhook", handler: func(s *Server) http.HandlerFunc { return s.getWebhook }},
	{path: "/api/v1/webhooks/{webhookID}", method: http.MethodPut, scope: "admin", tags: []string{"webhooks"}, operationID: "updateWebhook", requestBody: true, handler: func(s *Server) http.HandlerFunc { return s.updateWebhook }},
	{path: "/api/v1/webhooks/{webhookID}", method: http.MethodDelete, scope: "admin", tags: []string{"webhooks"}, operationID: "deleteWebhook", handler: func(s *Server) http.HandlerFunc { return s.deleteWebhook }},
	{path: "/api/v1/webhooks/{webhookID}/rotate", method: http.MethodPost, scope: "admin", tags: []string{"webhooks"}, operationID: "rotateWebhookSecret", successStatus: http.StatusOK, handler: func(s *Server) http.HandlerFunc { return s.rotateWebhookSecret }},
	{path: "/api/v1/webhooks/{webhookID}/deliveries", method: http.MethodGet, scope: "admin", tags: []string{"webhooks"}, operationID: "listWebhookDeliveries", handler: func(s *Server) http.HandlerFunc { return s.listWebhookDeliveries }},
}

func (server *Server) registerAPIRoutes(router *http.ServeMux) {
	for _, route := range server.availableAPIRoutes() {
		router.HandleFunc(route.method+" "+route.path, route.handler(server))
	}
}

func (server *Server) availableAPIRoutes() []apiRouteDefinition {
	routes := make([]apiRouteDefinition, 0, len(apiRouteDefinitions))
	for _, route := range apiRouteDefinitions {
		if route.requiresReplay && server.replay == nil {
			continue
		}
		routes = append(routes, route)
	}
	return routes
}

// corsMethods derives the browser-facing method list from the API manifest so a
// newly added mutation route cannot silently be blocked by CORS preflight.
func corsMethods(routes []apiRouteDefinition) string {
	allowed := map[string]bool{
		http.MethodGet:     true,
		http.MethodHead:    true,
		http.MethodOptions: true,
	}
	for _, route := range routes {
		allowed[route.method] = true
	}

	ordered := []string{
		http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodOptions,
	}
	methods := make([]string, 0, len(ordered))
	for _, method := range ordered {
		if allowed[method] {
			methods = append(methods, method)
		}
	}
	return strings.Join(methods, ", ")
}
