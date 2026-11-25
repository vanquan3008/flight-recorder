package projects

import (
	"testing"

	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render/rendertest"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

func TestResponsesKeepWireFormat(t *testing.T) {
	rendertest.AssertWireFormat(t, map[string][2]any{
		"created project": {createProjectResponse{Project: auth.Project{ID: "p"}, Environment: auth.Environment{ID: "e"}}, map[string]any{"project": auth.Project{ID: "p"}, "environment": auth.Environment{ID: "e"}}},
		"project detail":  {projectDetailResponse{Project: auth.Project{ID: "p"}, Role: "owner"}, map[string]any{"project": auth.Project{ID: "p"}, "role": "owner", "environments": []auth.Environment(nil)}},
	})
}
