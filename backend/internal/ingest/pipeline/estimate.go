package pipeline

import (
	"math"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

const (
	eventBaseBytes  int64 = 256
	mapEntryBytes   int64 = 16
	sliceEntryBytes int64 = 16
	spanEntryBytes  int64 = 32

	unknownValueBytes int64 = 32
	maxEstimateDepth        = 32
	maxEstimatedBytes       = int64(math.MaxInt64)
)

func estimateEventBytes(event telemetry.Event) int64 {
	total := eventBaseBytes

	total = addEstimatedBytes(total, int64(
		len(event.ID)+len(event.ProjectID)+len(event.Environment)+
			len(event.ServiceName)+len(event.ServiceVersion)+
			len(event.TraceID)+len(event.SpanID)+len(event.ParentSpanID)+
			len(event.RequestID)+len(event.Type)+len(event.Severity)+
			len(event.Status)+len(event.Operation)+len(event.ErrorType)+
			len(event.ErrorMessage)+len(event.ResourceSchemaURL)+
			len(event.ScopeName)+len(event.ScopeVersion)+
			len(event.ScopeSchemaURL)+len(event.TraceState)+
			len(event.StatusMessage)+len(event.LogEventName),
	))

	total = addEstimatedBytes(total, estimateMapBytes(event.Attributes))
	total = addEstimatedBytes(total, estimateMapBytes(event.ResourceAttributes))
	total = addEstimatedBytes(total, estimateMapBytes(event.ScopeAttributes))
	total = addEstimatedBytes(total, estimateMapBytes(event.RecordAttributes))

	for _, item := range event.SpanEvents {
		total = addEstimatedBytes(total, spanEntryBytes)
		total = addEstimatedBytes(total, int64(len(item.Name)))
		total = addEstimatedBytes(total, estimateMapBytes(item.Attributes))
		if total == maxEstimatedBytes {
			return total
		}
	}

	for _, item := range event.SpanLinks {
		total = addEstimatedBytes(total, spanEntryBytes)
		total = addEstimatedBytes(total, int64(len(item.TraceID)+len(item.SpanID)+len(item.TraceState)))
		total = addEstimatedBytes(total, estimateMapBytes(item.Attributes))
		if total == maxEstimatedBytes {
			return total
		}
	}

	if event.LogBody != nil {
		total = addEstimatedBytes(total, estimateValueBytes(event.LogBody, 0))
	}

	return total
}

func estimateMapBytes(attributes map[string]any) int64 {
	return estimateMapDepth(attributes, 0)
}
func estimateMapDepth(attributes map[string]any, depth int) int64 {
	if len(attributes) == 0 {
		return 0
	}
	if depth >= maxEstimateDepth {
		return unknownValueBytes
	}

	var total int64

	for key, value := range attributes {
		total = addEstimatedBytes(total, int64(len(key)))
		total = addEstimatedBytes(total, mapEntryBytes)
		total = addEstimatedBytes(total, estimateValueBytes(value, depth+1))
		if total == maxEstimatedBytes {
			return total
		}
	}

	return total
}
func estimateValueBytes(value any, depth int) int64 {
	switch v := value.(type) {
	case nil:
		return 0

	case string:
		return int64(len(v))

	case []byte:
		return int64(len(v))

	case bool, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return 8

	case map[string]any:
		return estimateMapDepth(v, depth)

	case []any:
		if len(v) == 0 {
			return 0
		}
		if depth >= maxEstimateDepth {
			return unknownValueBytes
		}

		total := multiplyEstimatedBytes(int64(len(v)), sliceEntryBytes)
		for _, item := range v {
			total = addEstimatedBytes(total, estimateValueBytes(item, depth+1))
			if total == maxEstimatedBytes {
				return total
			}
		}
		return total

	case []string:
		total := multiplyEstimatedBytes(int64(len(v)), sliceEntryBytes)
		for _, item := range v {
			total = addEstimatedBytes(total, int64(len(item)))
		}
		return total

	case [][]byte:
		total := multiplyEstimatedBytes(int64(len(v)), sliceEntryBytes)
		for _, item := range v {
			total = addEstimatedBytes(total, int64(len(item)))
		}
		return total

	case []map[string]any:
		if len(v) == 0 {
			return 0
		}
		if depth >= maxEstimateDepth {
			return unknownValueBytes
		}

		total := multiplyEstimatedBytes(int64(len(v)), sliceEntryBytes)
		for _, item := range v {
			total = addEstimatedBytes(total, estimateMapDepth(item, depth+1))
			if total == maxEstimatedBytes {
				return total
			}
		}
		return total

	default:
		return unknownValueBytes
	}
}

func addEstimatedBytes(a, b int64) int64 {
	if a >= maxEstimatedBytes-b {
		return maxEstimatedBytes
	}
	return a + b
}
func multiplyEstimatedBytes(a, b int64) int64 {
	if a > maxEstimatedBytes/b {
		return maxEstimatedBytes
	}
	return a * b
}
