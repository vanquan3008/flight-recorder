package analysis

import (
	"testing"

	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render/rendertest"

	"github.com/vanquan3008/tracepilot/internal/query"
)

func TestResponsesKeepWireFormat(t *testing.T) {
	rendertest.AssertWireFormat(t, map[string][2]any{
		"service dependencies": {serviceDependenciesResponse{Service: "api"}, map[string]any{"service": "api", "dependencies": []query.ServiceDependency(nil)}},
	})
}
