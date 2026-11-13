package openapi

import (
	"net/http"
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
