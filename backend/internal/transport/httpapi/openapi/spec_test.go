package openapi

import (
	"net/http"
	"reflect"
	"testing"
)

func TestSpecForRoutes(t *testing.T) {
	routes := []Route{
		{
			Path:          "/api/v1/test",
			Method:        http.MethodGet,
			Scope:         "read",
			Tags:          []string{"test"},
			OperationID:   "getTest",
			SuccessStatus: http.StatusOK,
		},
	}
	spec := SpecForRoutes(routes)
	if spec["openapi"] != "3.1.0" {
		t.Errorf("got %v, want 3.1.0", spec["openapi"])
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok || paths["/api/v1/test"] == nil {
		t.Error("missing path /api/v1/test in spec")
	}
}

type itemBody struct {
	Name string `json:"name"`
}

func TestSpecRendersRouteMetadata(t *testing.T) {
	spec := SpecForRoutes([]Route{
		{Path: "/api/v1/items", Method: http.MethodGet, Scope: "read", OperationID: "listItems", Query: []string{"Limit", "Cursor"}, Headers: []string{"ProjectHeader"}, Response: Page[itemBody]()},
		{Path: "/api/v1/items", Method: http.MethodPost, Scope: "admin", OperationID: "createItem", RequestBody: true, Request: TypeOf[itemBody](), Response: Object[itemBody]()},
		{Path: "/api/v1/items/{itemID}", Method: http.MethodDelete, Scope: "admin", OperationID: "deleteItem", Response: Empty()},
		{Path: "/api/v1/stream", Method: http.MethodGet, Scope: "read", OperationID: "streamItems", Response: EventStream()},
		{Path: "/api/v1/status", Method: http.MethodGet, Scope: "public", Public: true, OperationID: "getStatus", Response: Object[itemBody]()},
	})
	paths := spec["paths"].(map[string]any)
	operation := func(path, method string) map[string]any {
		return paths[path].(map[string]any)[method].(map[string]any)
	}

	list := operation("/api/v1/items", "get")
	wantParameters := []map[string]any{
		{"$ref": "#/components/parameters/ProjectHeader"},
		{"$ref": "#/components/parameters/Limit"},
		{"$ref": "#/components/parameters/Cursor"},
	}
	if !reflect.DeepEqual(list["parameters"], wantParameters) {
		t.Fatalf("list parameters = %v", list["parameters"])
	}
	if got := list["security"]; !reflect.DeepEqual(got, []map[string][]string{{"BearerAuth": {"read"}}}) {
		t.Fatalf("list security = %v", got)
	}
	content := list["responses"].(map[string]any)["200"].(map[string]any)["content"].(map[string]any)
	if content["application/json"].(map[string]any)["schema"].(map[string]any)["$ref"] != "#/components/schemas/ItemBodyPage" {
		t.Fatalf("list body = %v", content)
	}

	create := operation("/api/v1/items", "post")
	body := create["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"]
	if !reflect.DeepEqual(body, map[string]any{"$ref": "#/components/schemas/ItemBody"}) {
		t.Fatalf("request body schema = %v", body)
	}
	if _, ok := create["responses"].(map[string]any)["201"]; !ok {
		t.Fatal("POST must default to 201")
	}

	remove := operation("/api/v1/items/{itemID}", "delete")["responses"].(map[string]any)
	if noContent := remove["204"].(map[string]any); noContent["content"] != nil {
		t.Fatalf("204 must not have content: %v", noContent)
	}
	if remove["404"] == nil {
		t.Fatal("routes with path parameters must document 404")
	}

	stream := operation("/api/v1/stream", "get")["responses"].(map[string]any)
	if _, ok := stream["200"].(map[string]any)["content"].(map[string]any)["text/event-stream"]; !ok || stream["504"] != nil {
		t.Fatalf("stream responses = %v", stream)
	}

	status := operation("/api/v1/status", "get")
	if len(status["security"].([]map[string][]string)) != 0 || status["responses"].(map[string]any)["403"] != nil {
		t.Fatalf("public route = %v", status)
	}
	components := spec["components"].(map[string]any)
	for _, group := range []string{"schemas", "parameters", "responses", "securitySchemes"} {
		if len(components[group].(map[string]any)) == 0 {
			t.Errorf("components.%s is empty", group)
		}
	}
}

func TestRouteSummary(t *testing.T) {
	for operationID, want := range map[string]string{
		"listTraceLogs":  "List trace logs",
		"getCurrentUser": "Get current user",
		"login":          "Login",
		"":               "TracePilot management operation",
	} {
		if got := RouteSummary(operationID); got != want {
			t.Errorf("RouteSummary(%q) = %q, want %q", operationID, got, want)
		}
	}
}

func TestRouteParametersAndResponses(t *testing.T) {
	parameters := RouteParameters(Route{Path: "/api/v1/traces/{traceID}/spans/{spanID}", OperationID: "getTrace"})
	if len(parameters) != 2 || parameters[0]["name"] != "traceID" || parameters[0]["schema"].(map[string]any)["format"] != nil {
		t.Fatalf("path parameters = %v", parameters)
	}
	if parameters[1]["schema"].(map[string]any)["format"] != "uuid" {
		t.Fatalf("IDs other than trace IDs are UUIDs: %v", parameters[1])
	}
	responses := routeResponses(Route{Method: http.MethodPost, Path: "/api/v1/projects"}, newSchemas())
	if responses["201"] == nil || !reflect.DeepEqual(responses["401"], map[string]any{"$ref": "#/components/responses/Unauthorized"}) {
		t.Fatalf("responses = %v", responses)
	}
	if responses := routeResponses(Route{Method: http.MethodPost, Path: "/api/v1/projects", SuccessStatus: http.StatusOK}, newSchemas()); responses["200"] == nil || responses["201"] != nil {
		t.Fatalf("configured status must win: %v", responses)
	}
	if got := OpenAPISecurity(Route{Public: true}); len(got) != 0 {
		t.Fatalf("public security = %v", got)
	}
}
