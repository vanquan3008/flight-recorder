package httpapi

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
)

func TestOpenAPIRoutesDeclareBodiesAndKnownParameters(t *testing.T) {
	for _, route := range apiRouteDefinitions {
		key := route.method + " " + route.path
		if route.response.Kind == openapi.BodyUntyped && !route.streaming {
			t.Errorf("%s declares no response body", key)
		}
		if route.requestBody && route.request == nil {
			t.Errorf("%s accepts a JSON body but declares no request type", key)
		}
		if route.request != nil && !route.requestBody {
			t.Errorf("%s declares a request type without requestBody", key)
		}
		for _, name := range slices.Concat(route.query, route.headers) {
			if !openapi.HasParameter(name) {
				t.Errorf("%s names unknown parameter %q; known: %v", key, name, openapi.ParameterNames())
			}
		}
	}
}

// TestOpenAPIReferencesResolve serializes the document as clients receive it
// and checks that every $ref names an existing component.
func TestOpenAPIReferencesResolve(t *testing.T) {
	encoded, err := json.Marshal(openAPISpec())
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
	spec := openAPISpec()
	traces := parameterSet(spec, "/api/v1/traces", "get")
	if traces["header:"+HeaderProjectID]["required"] != true || traces["header:"+HeaderEnvironment] == nil {
		t.Fatalf("trace list must document %s (required) and %s: %v", HeaderProjectID, HeaderEnvironment, traces)
	}
	project := parameterSet(spec, "/api/v1/projects/{projectID}", "get")
	if project["header:"+HeaderProjectID] != nil || project["header:"+HeaderEnvironment] == nil || project["path:projectID"] == nil {
		t.Fatalf("a route naming the project in its path must not require %s: %v", HeaderProjectID, project)
	}
	if projects := parameterSet(spec, "/api/v1/projects", "get"); len(projects) != 0 {
		t.Fatalf("session routes take no project headers: %v", projects)
	}
	refresh := parameterSet(spec, "/api/v1/auth/refresh", "post")
	if refresh["header:"+refreshHeader] == nil || refresh["cookie:"+refreshCookieName] == nil {
		t.Fatalf("refresh must document %s and the %s cookie: %v", refreshHeader, refreshCookieName, refresh)
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
	spec := openAPISpec()
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

// TestTypedResponsesKeepWireFormat proves the response types that replaced
// map literals encode exactly the same JSON.
func TestTypedResponsesKeepWireFormat(t *testing.T) {
	cases := []struct {
		name  string
		typed any
		old   any
	}{
		{"auth status", authStatusResponse{RegistrationOpen: true}, map[string]any{"registration_open": true, "first_account": false}},
		{"trace without more pages", traceResponse{TraceID: "t1", Spans: []query.SpanRecord{}}, map[string]any{"trace_id": "t1", "spans": []query.SpanRecord{}}},
		{"trace with cursor", traceResponse{TraceID: "t1", Spans: []query.SpanRecord{}, NextCursor: "c"}, map[string]any{"trace_id": "t1", "spans": []query.SpanRecord{}, "next_cursor": "c"}},
		{"service dependencies", serviceDependenciesResponse{Service: "api"}, map[string]any{"service": "api", "dependencies": []query.ServiceDependency(nil)}},
		{"created project", createProjectResponse{Project: auth.Project{ID: "p"}, Environment: auth.Environment{ID: "e"}}, map[string]any{"project": auth.Project{ID: "p"}, "environment": auth.Environment{ID: "e"}}},
		{"project detail", projectDetailResponse{Project: auth.Project{ID: "p"}, Role: "owner"}, map[string]any{"project": auth.Project{ID: "p"}, "role": "owner", "environments": []auth.Environment(nil)}},
		{"member role", memberRoleResponse{UserID: "u", Role: "user"}, map[string]any{"user_id": "u", "role": "user"}},
		{"issued token", issuedIngestTokenResponse{JWT: "j"}, map[string]any{"token": auth.IngestToken{}, "jwt": "j"}},
		{"created user without generated password", createUserResponse{User: auth.User{ID: "u"}}, map[string]any{"user": auth.User{ID: "u"}}},
		{"created user with generated password", createUserResponse{User: auth.User{ID: "u"}, TemporaryPassword: "pw"}, map[string]any{"user": auth.User{ID: "u"}, "temporary_password": "pw"}},
		{"temporary password", temporaryPasswordResponse{TemporaryPassword: "pw"}, map[string]any{"temporary_password": "pw"}},
		{"webhook secret", webhookSecretResponse{Secret: "s"}, map[string]string{"secret": "s"}},
	}
	for _, tc := range cases {
		typed, typedError := json.Marshal(tc.typed)
		old, oldError := json.Marshal(tc.old)
		if typedError != nil || oldError != nil {
			t.Fatalf("%s: marshal errors %v %v", tc.name, typedError, oldError)
		}
		if string(typed) != string(old) {
			t.Errorf("%s: typed %s, previous %s", tc.name, typed, old)
		}
	}
}
