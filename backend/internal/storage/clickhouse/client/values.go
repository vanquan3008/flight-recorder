package client

import (
	"encoding/json"
	"fmt"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

// JSONText encodes a value as JSON text for a String column.
func JSONText(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode log value: %w", telemetry.ErrInvalidEvent)
	}
	return string(data), nil
}

// AttributeMap stores attribute values as text, as the OpenTelemetry ClickHouse
// exporter does: strings as they are, other values as JSON, null as "".
func AttributeMap(values map[string]any) (map[string]string, error) {
	result := make(map[string]string, len(values))
	for key, value := range values {
		switch typed := value.(type) {
		case nil:
			result[key] = ""
		case string:
			result[key] = typed
		default:
			encoded, err := json.Marshal(typed)
			if err != nil {
				return nil, fmt.Errorf("encode attribute: %w", telemetry.ErrInvalidEvent)
			}
			result[key] = string(encoded)
		}
	}
	return result, nil
}

// AttributeJSON encodes a string map as a JSON object, never null.
func AttributeJSON(values map[string]string) json.RawMessage {
	if values == nil {
		values = map[string]string{}
	}
	encoded, _ := json.Marshal(values) // a string map always encodes
	return encoded
}

// JSONArray encodes a slice as a JSON array, with [] for nil.
func JSONArray[T any](values []T) (string, error) {
	if values == nil {
		return "[]", nil
	}
	return JSONText(values)
}
