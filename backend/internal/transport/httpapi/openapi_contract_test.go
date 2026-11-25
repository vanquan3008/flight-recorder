package httpapi

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
)

func TestOpenAPIRoutesDeclareBodiesAndKnownParameters(t *testing.T) {
	for _, route := range allRoutes() {
		key := route.Method + " " + route.Path
		if route.Response.Kind == openapi.BodyUntyped && !route.Streaming {
			t.Errorf("%s declares no response body", key)
		}
		if route.RequestBody && route.Request == nil {
			t.Errorf("%s accepts a JSON body but declares no request type", key)
		}
		if route.Request != nil && !route.RequestBody {
			t.Errorf("%s declares a request type without requestBody", key)
		}
		for _, name := range slices.Concat(route.Query, route.Headers) {
			if !openapi.HasParameter(name) {
				t.Errorf("%s names unknown parameter %q", key, name)
			}
		}
	}
}

// TestOpenAPIReferencesResolve serializes the document as clients receive it
// and checks that every $ref names an existing component.
func TestOpenAPIReferencesResolve(t *testing.T) {
	encoded, err := json.Marshal(testSpec())
	if err != nil {
		t.Fatalf("marshal OpenAPI: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("unmarshal OpenAPI: %v", err)
	}
	components := document["components"].(map[string]any)
	references := 0
	var walk func(value any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			if reference, ok := typed["$ref"].(string); ok {
				references++
				parts := strings.Split(strings.TrimPrefix(reference, "#/components/"), "/")
				group, _ := components[parts[0]].(map[string]any)
				if len(parts) != 2 || group[parts[1]] == nil {
					t.Errorf("unresolved reference %s", reference)
				}
			}
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(document)
	if references == 0 {
		t.Fatal("OpenAPI document has no references")
	}
}

func parameterSet(spec map[string]any, path, method string) map[string]map[string]any {
	set := map[string]map[string]any{}
	for _, parameter := range operationParameters(spec, path, method) {
		set[parameter["in"].(string)+":"+parameter["name"].(string)] = parameter
	}
	return set
}

func TestOpenAPIDocumentsProjectHeadersAndScopes(t *testing.T) {
	spec := testSpec()
	traces := parameterSet(spec, "/api/v1/traces", "get")
	if traces["header:"+api.HeaderProjectID]["required"] != true || traces["header:"+api.HeaderEnvironment] == nil {
		t.Fatalf("trace list must document %s (required) and %s: %v", api.HeaderProjectID, api.HeaderEnvironment, traces)
	}
	project := parameterSet(spec, "/api/v1/projects/{projectID}", "get")
	if project["header:"+api.HeaderProjectID] != nil || project["header:"+api.HeaderEnvironment] == nil || project["path:projectID"] == nil {
		t.Fatalf("a route naming the project in its path must not require %s: %v", api.HeaderProjectID, project)
	}
	if projects := parameterSet(spec, "/api/v1/projects", "get"); len(projects) != 0 {
		t.Fatalf("session routes take no project headers: %v", projects)
	}
	refresh := parameterSet(spec, "/api/v1/auth/refresh", "post")
	if refresh["header:X-TracePilot-Refresh"] == nil || refresh["cookie:tp_refresh"] == nil {
		t.Fatalf("refresh must document X-TracePilot-Refresh and the tp_refresh cookie: %v", refresh)
	}

	paths := spec["paths"].(map[string]any)
	operation := func(path, method string) map[string]any {
		return paths[path].(map[string]any)[method].(map[string]any)
	}
	if security := operation("/api/v1/auth/login", "post")["security"].([]map[string][]string); len(security) != 0 {
		t.Fatalf("public login must not require credentials: %v", security)
	}
	security := operation("/api/v1/traces", "get")["security"].([]map[string][]string)
	if len(security) != 1 || !slices.Equal(security[0]["BearerAuth"], []string{auth.ScopeRead}) {
		t.Fatalf("trace list security = %v, want BearerAuth with scope read", security)
	}
	stream := operation("/api/v1/live/stream", "get")["responses"].(map[string]any)["200"].(map[string]any)
	if _, ok := stream["content"].(map[string]any)["text/event-stream"]; !ok {
		t.Fatalf("live stream must document text/event-stream: %v", stream)
	}
}

func TestOpenAPIPublishesRequestAndResponseSchemas(t *testing.T) {
	spec := testSpec()
	paths := spec["paths"].(map[string]any)
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)
	schemaRef := func(content any) string {
		return content.(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)["$ref"].(string)
	}
	createProject := paths["/api/v1/projects"].(map[string]any)["post"].(map[string]any)
	if got := schemaRef(createProject["requestBody"].(map[string]any)["content"]); got != "#/components/schemas/CreateProjectRequest" {
		t.Fatalf("createProject request schema = %s", got)
	}
	properties := schemas["CreateProjectRequest"].(map[string]any)["properties"].(map[string]any)
	for _, name := range []string{"slug", "name", "environment"} {
		if properties[name] == nil {
			t.Fatalf("CreateProjectRequest is missing %q: %v", name, properties)
		}
	}

	login := paths["/api/v1/auth/login"].(map[string]any)["post"].(map[string]any)["responses"].(map[string]any)
	if got := schemaRef(login["200"].(map[string]any)["content"]); got != "#/components/schemas/SessionView" {
		t.Fatalf("login response schema = %s", got)
	}
	traces := paths["/api/v1/traces"].(map[string]any)["get"].(map[string]any)["responses"].(map[string]any)
	if got := schemaRef(traces["200"].(map[string]any)["content"]); got != "#/components/schemas/TraceSummaryPage" {
		t.Fatalf("trace list response schema = %s", got)
	}
	page := schemas["TraceSummaryPage"].(map[string]any)["properties"].(map[string]any)
	if page["next_cursor"] == nil || page["data"].(map[string]any)["items"].(map[string]any)["$ref"] != "#/components/schemas/TraceSummary" {
		t.Fatalf("TraceSummaryPage = %v", page)
	}
	logout := paths["/api/v1/auth/logout"].(map[string]any)["post"].(map[string]any)["responses"].(map[string]any)
	if noContent, ok := logout["204"].(map[string]any); !ok || noContent["content"] != nil {
		t.Fatalf("logout must answer 204 without content: %v", logout)
	}
	if _, ok := traces["504"]; !ok {
		t.Fatal("bounded query routes must document 504")
	}
	if _, ok := login["403"]; ok {
		t.Fatal("public routes do not return 403")
	}
}
