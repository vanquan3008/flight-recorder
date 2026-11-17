package openapi

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"
)

type sampleInner struct {
	Value int `json:"value"`
}

type sampleBase struct {
	ID string `json:"id"`
}

type sample struct {
	sampleBase
	Name     string            `json:"name"`
	Optional string            `json:"optional,omitempty"`
	When     time.Time         `json:"when"`
	Maybe    *string           `json:"maybe"`
	Inner    sampleInner       `json:"inner"`
	Items    []sampleInner     `json:"items"`
	Labels   map[string]string `json:"labels"`
	Raw      json.RawMessage   `json:"raw"`
	Count    int64             `json:"count,string"`
	Hidden   string            `json:"-"`
	hidden   string
	Untagged int
	Self     *sample `json:"self,omitempty"`
}

// ErrorResponse collides with the hand-written ErrorResponse component.
type ErrorResponse struct {
	Code string `json:"code"`
}

func TestSchemaFollowsEncodingJSON(t *testing.T) {
	registry := newSchemas()
	reference := registry.schemaFor(reflect.TypeFor[sample]())
	if reference["$ref"] != "#/components/schemas/Sample" {
		t.Fatalf("named struct reference = %v", reference)
	}
	_ = sample{}.hidden
	component := registry.components["Sample"].(map[string]any)
	properties := component["properties"].(map[string]any)
	required := component["required"].([]string)

	for _, name := range []string{"id", "name", "when", "inner", "items", "labels", "raw", "count", "Untagged"} {
		if properties[name] == nil {
			t.Errorf("missing property %q", name)
		}
		if !slices.Contains(required, name) {
			t.Errorf("property %q must be required", name)
		}
	}
	for _, name := range []string{"optional", "maybe", "self"} {
		if properties[name] == nil || slices.Contains(required, name) {
			t.Errorf("property %q must exist and be optional", name)
		}
	}
	for _, name := range []string{"Hidden", "-", "hidden"} {
		if properties[name] != nil {
			t.Errorf("property %q must not be published", name)
		}
	}
	assertEqual(t, "when", properties["when"], map[string]any{"type": "string", "format": "date-time"})
	assertEqual(t, "maybe", properties["maybe"], map[string]any{"type": []string{"string", "null"}})
	assertEqual(t, "count", properties["count"], map[string]any{"type": "string"})
	assertEqual(t, "raw", properties["raw"], map[string]any{})
	assertEqual(t, "inner", properties["inner"], map[string]any{"$ref": "#/components/schemas/SampleInner"})
	assertEqual(t, "self", properties["self"], map[string]any{"$ref": "#/components/schemas/Sample"})
	assertEqual(t, "labels", properties["labels"], map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}})
	assertEqual(t, "items", properties["items"], map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/SampleInner"}})
}

func TestSchemaQualifiesTypesThatClaimATakenName(t *testing.T) {
	registry := newSchemas()
	addErrorSchemas(registry)
	reference := registry.schemaFor(reflect.TypeFor[ErrorResponse]())
	if reference["$ref"] != "#/components/schemas/OpenapiErrorResponse" {
		t.Fatalf("colliding type reference = %v", reference)
	}
	if registry.components["ErrorResponse"].(map[string]any)["required"].([]string)[0] != "error" {
		t.Fatal("the hand-written ErrorResponse schema was replaced")
	}
}

func TestEnvelopesNameTheItemType(t *testing.T) {
	registry := newSchemas()
	page := registry.envelope(reflect.TypeFor[sampleInner](), "Page", true)
	list := registry.envelope(reflect.TypeFor[sampleInner](), "List", false)
	if page["$ref"] != "#/components/schemas/SampleInnerPage" || list["$ref"] != "#/components/schemas/SampleInnerList" {
		t.Fatalf("envelope references = %v, %v", page, list)
	}
	pageProperties := registry.components["SampleInnerPage"].(map[string]any)["properties"].(map[string]any)
	listProperties := registry.components["SampleInnerList"].(map[string]any)["properties"].(map[string]any)
	if pageProperties["next_cursor"] == nil || listProperties["next_cursor"] != nil {
		t.Fatalf("only pages carry next_cursor: page=%v list=%v", pageProperties, listProperties)
	}
}

func assertEqual(t *testing.T, name string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s schema = %#v, want %#v", name, got, want)
	}
}
