package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vanquan3008/tracepilot/internal/application/replay"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/features/traces"
)

func TestServerExposesOpenAPIContract(t *testing.T) {
	httpServer := newTestServer(func(context.Context) error { return nil })
	responseRecorder := httptest.NewRecorder()
	httpServer.httpServer.Handler.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status = %d", responseRecorder.Code)
	}
	if !strings.Contains(responseRecorder.Body.String(), `"openapi":"3.1.0"`) || !strings.Contains(responseRecorder.Body.String(), `"/api/v1/projects/{projectID}/ingest-tokens"`) || !strings.Contains(responseRecorder.Body.String(), `"/api/v1/incidents/{incidentID}/analysis"`) {
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
	spec := testSpec()
	paths := spec["paths"].(map[string]any)
	assertStatus := func(path, method, want string) {
		t.Helper()
		responses := paths[path].(map[string]any)[method].(map[string]any)["responses"].(map[string]any)
		if _, ok := responses[want]; !ok {
			t.Fatalf("%s %s does not advertise %s response", method, path, want)
		}
	}
	assertStatus("/api/v1/auth/refresh", "post", "200")
	assertStatus("/api/v1/webhooks/{webhookID}/rotate", "post", "200")
	assertStatus("/api/v1/traces/{traceID}", "get", "200")

	// A trace is read in pages so one request never loads a whole large trace.
	parameters := operationParameters(spec, "/api/v1/traces/{traceID}", "get")
	advertised := map[string]bool{}
	for _, parameter := range parameters {
		if parameter["in"] == "query" {
			advertised[parameter["name"].(string)] = true
			if parameter["name"] == "limit" && parameter["schema"].(map[string]any)["maximum"] != traces.MaxSpansPerPage {
				t.Fatalf("getTrace limit maximum = %v, want %d", parameter["schema"].(map[string]any)["maximum"], traces.MaxSpansPerPage)
			}
		}
	}
	if !advertised["limit"] || !advertised["cursor"] || len(advertised) != 2 {
		t.Fatalf("getTrace query parameters = %v, want limit and cursor", advertised)
	}
}

func TestOpenAPIQueryParametersMatchHandlerFilters(t *testing.T) {
	spec := testSpec()
	assertParameters := func(path, method string, want ...string) {
		t.Helper()
		parameters := operationParameters(spec, path, method)
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
	paths := testSpec()["paths"].(map[string]any)
	if len(paths) == 0 {
		t.Fatal("OpenAPI has no API paths")
	}
	routes := allRoutes()
	seen := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		key := route.Method + " " + route.Path
		if _, exists := seen[key]; exists {
			t.Fatalf("duplicate API route %s", key)
		}
		seen[key] = struct{}{}
		operations, ok := paths[route.Path].(map[string]any)
		if !ok {
			t.Fatalf("OpenAPI is missing path %s", route.Path)
		}
		if _, ok := operations[strings.ToLower(route.Method)]; !ok {
			t.Fatalf("OpenAPI is missing operation %s", key)
		}
	}
	dependencies := testDependencies()
	dependencies.Replay = replayRunner{}
	router := apiRouter(New(testConfig(), dependencies))
	for _, route := range routes {
		path := strings.NewReplacer(
			"{traceID}", "0123456789abcdef0123456789abcdef",
			"{eventID}", "01900000-0000-7000-8000-000000000001",
			"{ruleID}", "01900000-0000-7000-8000-000000000002",
			"{incidentID}", "01900000-0000-7000-8000-000000000003",
			"{executionID}", "01900000-0000-7000-8000-000000000004",
			"{keyID}", "01900000-0000-7000-8000-000000000005",
			"{webhookID}", "01900000-0000-7000-8000-000000000006",
			"{service}", "checkout",
		).Replace(route.Path)
		request := httptest.NewRequest(route.Method, path, nil)
		_, pattern := router.Handler(request)
		if pattern != route.Method+" "+route.Path {
			t.Fatalf("router pattern for %s = %q", route.Method+" "+route.Path, pattern)
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

// replayRunner makes the server mount the replay route, so the tests see every route.
type replayRunner struct{}

func (replayRunner) Run(context.Context, replay.Command) (replay.Outcome, error) {
	return replay.Outcome{}, nil
}

// allRoutes are the routes of a server with every optional dependency.
func allRoutes() []api.Route {
	dependencies := testDependencies()
	dependencies.Replay = replayRunner{}
	return New(testConfig(), dependencies).routes
}

func testSpec() map[string]any {
	return openAPISpec(allRoutes())
}

// operationParameters returns an operation's parameters with references to
// components/parameters replaced by their definitions.
func operationParameters(spec map[string]any, path, method string) []map[string]any {
	operation := spec["paths"].(map[string]any)[path].(map[string]any)[method].(map[string]any)
	shared := spec["components"].(map[string]any)["parameters"].(map[string]any)
	var resolved []map[string]any
	for _, parameter := range operation["parameters"].([]map[string]any) {
		if reference, ok := parameter["$ref"].(string); ok {
			parameter = shared[strings.TrimPrefix(reference, "#/components/parameters/")].(map[string]any)
		}
		resolved = append(resolved, parameter)
	}
	return resolved
}
