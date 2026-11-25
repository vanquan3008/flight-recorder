package ingesttokens

import (
	"testing"

	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render/rendertest"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

func TestResponsesKeepWireFormat(t *testing.T) {
	rendertest.AssertWireFormat(t, map[string][2]any{
		"issued token": {issuedIngestTokenResponse{JWT: "j"}, map[string]any{"token": auth.IngestToken{}, "jwt": "j"}},
	})
}
