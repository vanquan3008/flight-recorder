package webhooks

import (
	"testing"

	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render/rendertest"
)

func TestResponsesKeepWireFormat(t *testing.T) {
	rendertest.AssertWireFormat(t, map[string][2]any{
		"webhook secret": {webhookSecretResponse{Secret: "s"}, map[string]string{"secret": "s"}},
	})
}
