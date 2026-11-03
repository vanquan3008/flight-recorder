package pipeline

import (
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

// estimateEventBytes provides a fast, zero-reflection, zero-allocation byte estimate
// for telemetry.Event to avoid running json.NewEncoder on the ingestion hot-path.
func estimateEventBytes(event telemetry.Event) int64 {
	total := int64(256)

	total += int64(len(event.ID) + len(event.ProjectID) + len(event.Environment) +
		len(event.ServiceName) + len(event.ServiceVersion) + len(event.TraceID) +
		len(event.SpanID) + len(event.ParentSpanID) + len(event.RequestID) +
		len(event.Type) + len(event.Severity) + len(event.Status) +
		len(event.Operation) + len(event.ErrorType) + len(event.ErrorMessage) +
		len(event.ResourceSchemaURL) + len(event.ScopeName) + len(event.ScopeVersion) +
		len(event.ScopeSchemaURL) + len(event.TraceState) + len(event.StatusMessage) +
		len(event.LogEventName))

	total += estimateMapBytes(event.Attributes)
	total += estimateMapBytes(event.ResourceAttributes)
	total += estimateMapBytes(event.ScopeAttributes)
	total += estimateMapBytes(event.RecordAttributes)

	for _, spanEvent := range event.SpanEvents {
		total += int64(32+len(spanEvent.Name)) + estimateMapBytes(spanEvent.Attributes)
	}
	for _, spanLink := range event.SpanLinks {
		total += int64(32+len(spanLink.TraceID)+len(spanLink.SpanID)+len(spanLink.TraceState)) + estimateMapBytes(spanLink.Attributes)
	}

	if event.LogBody != nil {
		switch body := event.LogBody.(type) {
		case string:
			total += int64(len(body))
		case []byte:
			total += int64(len(body))
		case map[string]any:
			total += estimateMapBytes(body)
		default:
			total += 64
		}
	}

	return total
}

func estimateMapBytes(attributes map[string]any) int64 {
	if len(attributes) == 0 {
		return 0
	}
	var total int64
	for key, value := range attributes {
		total += int64(len(key)) + 16
		switch val := value.(type) {
		case string:
			total += int64(len(val))
		case []byte:
			total += int64(len(val))
		case int, int64, int32, float64, float32, bool:
			total += 8
		case map[string]any:
			total += estimateMapBytes(val)
		case []any:
			total += int64(len(val)) * 16
		default:
			total += 32
		}
	}
	return total
}
