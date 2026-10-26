package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerExposesOpenAPIContract(t *testing.T) {
	httpServer := newTestServer(func(context.Context) error { return nil })
	responseRecorder := httptest.NewRecorder()
	httpServer.httpServer.Handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status = %d", responseRecorder.Code)
	}
	if !strings.Contains(responseRecorder.Body.String(), `"openapi":"3.1.0"`) || !strings.Contains(responseRecorder.Body.String(), `"/api/v1/api-key-events"`) || !strings.Contains(responseRecorder.Body.String(), `"/api/v1/incidents/{incidentID}/analysis"`) {
		t.Fatalf("unexpected OpenAPI response: %s", responseRecorder.Body.String())
	}
}

func TestOpenAPIOmitsDisabledReplayRoutes(t *testing.T) {
	httpServer := newTestServer(func(context.Context) error { return nil })
	responseRecorder := httptest.NewRecorder()
	httpServer.httpServer.Handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))

	if strings.Contains(responseRecorder.Body.String(), `"/api/v1/incidents/{incidentID}/replays"`) {
		t.Fatalf("OpenAPI advertises replay while it is disabled: %s", responseRecorder.Body.String())
	}
}

func TestOpenAPIUsesHandlerSuccessStatusCodes(t *testing.T) {
	paths := openAPISpec()["paths"].(map[string]any)
	assertStatus := func(path, method, want string) {
		t.Helper()
		responses := paths[path].(map[string]any)[method].(map[string]any)["responses"].(map[string]any)
		if _, ok := responses[want]; !ok {
			t.Fatalf("%s %s does not advertise %s response", method, path, want)
		}
	}
	assertStatus("/api/v1/api-keys/{keyID}/rotate", "post", "200")
	assertStatus("/api/v1/webhooks/{webhookID}/rotate", "post", "200")
	assertStatus("/api/v1/traces/{traceID}", "get", "200")

	parameters := paths["/api/v1/traces/{traceID}"].(map[string]any)["get"].(map[string]any)["parameters"].([]map[string]any)
	for _, parameter := range parameters {
		if parameter["name"] == "limit" || parameter["name"] == "cursor" {
			t.Fatalf("getTrace should not advertise pagination parameter %q", parameter["name"])
		}
	}
}

func TestOpenAPIQueryParametersMatchHandlerFilters(t *testing.T) {
	paths := openAPISpec()["paths"].(map[string]any)
	assertParameters := func(path, method string, want ...string) {
		t.Helper()
		operation := paths[path].(map[string]any)[method].(map[string]any)
		parameters := operation["parameters"].([]map[string]any)
		got := make(map[string]bool, len(parameters))
		for _, parameter := range parameters {
			if parameter["in"] == "query" {
				got[parameter["name"].(string)] = true
			}
		}
		if len(got) != len(want) {
			t.Fatalf("%s %s query parameters = %v, want %v", method, path, got, want)
		}
		for _, name := range want {
			if !got[name] {
				t.Fatalf("%s %s is missing query parameter %q", method, path, name)
			}
		}
	}

	assertParameters("/api/v1/traces", "get", "limit", "cursor", "from", "to", "service", "operation", "status", "traceId", "minDuration", "maxDuration")
	assertParameters("/api/v1/logs", "get", "limit", "cursor", "from", "to", "service", "severity", "traceId", "spanId", "search")
	assertParameters("/api/v1/incidents", "get", "limit", "cursor", "from", "to", "service", "severity", "status")
	assertParameters("/api/v1/topology", "get", "from", "to", "service")
	assertParameters("/api/v1/metrics/query", "get", "limit", "from", "to", "service", "metric")
	assertParameters("/api/v1/services", "get", "from", "to")
	assertParameters("/api/v1/performance", "get", "limit", "from", "to", "service", "endpoint")
}

func TestOpenAPIRoutesComeFromRouterManifest(t *testing.T) {
	paths := openAPISpec()["paths"].(map[string]any)
	if len(paths) == 0 {
		t.Fatal("OpenAPI has no API paths")
	}
	seen := make(map[string]struct{}, len(apiRouteDefinitions))
	for _, route := range apiRouteDefinitions {
		key := route.method + " " + route.path
		if _, exists := seen[key]; exists {
			t.Fatalf("duplicate API route %s", key)
		}
		seen[key] = struct{}{}
		operations, ok := paths[route.path].(map[string]any)
		if !ok {
			t.Fatalf("OpenAPI is missing path %s", route.path)
		}
		if _, ok := operations[strings.ToLower(route.method)]; !ok {
			t.Fatalf("OpenAPI is missing operation %s", key)
		}
	}
	router := http.NewServeMux()
	server := &Server{}
	server.registerAPIRoutes(router)
	for _, route := range apiRouteDefinitions {
		if route.requiresReplay {
			continue
		}
		path := strings.NewReplacer(
			"{traceID}", "0123456789abcdef0123456789abcdef",
			"{eventID}", "01900000-0000-7000-8000-000000000001",
			"{ruleID}", "01900000-0000-7000-8000-000000000002",
			"{incidentID}", "01900000-0000-7000-8000-000000000003",
			"{executionID}", "01900000-0000-7000-8000-000000000004",
			"{keyID}", "01900000-0000-7000-8000-000000000005",
			"{webhookID}", "01900000-0000-7000-8000-000000000006",
			"{service}", "checkout",
		).Replace(route.path)
		request := httptest.NewRequest(route.method, path, nil)
		_, pattern := router.Handler(request)
		if pattern != route.method+" "+route.path {
			t.Fatalf("router pattern for %s = %q", route.method+" "+route.path, pattern)
		}
	}
	if len(seen) != countOpenAPIOperations(paths) {
		t.Fatalf("manifest routes=%d OpenAPI operations=%d", len(seen), countOpenAPIOperations(paths))
	}
}

func countOpenAPIOperations(paths map[string]any) int {
	count := 0
	for _, raw := range paths {
		for method := range raw.(map[string]any) {
			if method != "parameters" {
				count++
			}
		}
	}
	return count
}
