package members

import (
	"testing"

	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render/rendertest"
)

func TestResponsesKeepWireFormat(t *testing.T) {
	rendertest.AssertWireFormat(t, map[string][2]any{
		"member role": {memberRoleResponse{UserID: "u", Role: "user"}, map[string]any{"user_id": "u", "role": "user"}},
	})
}
