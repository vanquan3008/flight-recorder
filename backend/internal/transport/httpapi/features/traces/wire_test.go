package traces

import (
	"testing"

	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render/rendertest"

	"github.com/vanquan3008/tracepilot/internal/query"
)

func TestResponsesKeepWireFormat(t *testing.T) {
	rendertest.AssertWireFormat(t, map[string][2]any{
		"trace without more pages": {traceResponse{TraceID: "t1", Spans: []query.SpanRecord{}}, map[string]any{"trace_id": "t1", "spans": []query.SpanRecord{}}},
		"trace with cursor":        {traceResponse{TraceID: "t1", Spans: []query.SpanRecord{}, NextCursor: "c"}, map[string]any{"trace_id": "t1", "spans": []query.SpanRecord{}, "next_cursor": "c"}},
	})
}
