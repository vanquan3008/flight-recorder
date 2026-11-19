package telemetry

import (
	"encoding/json"
	"strings"
)

// These span rules derive the endpoint, request failure, and dependency that
// ClickHouse stores with each span. They read only the span's own attributes.

const spanKindClient = 3

func SpanEndpoint(attributes map[string]any, name string) string {
	if route := attributeText(attributes, "http.route"); route != "" {
		method := attributeText(attributes, "http.request.method")
		if method == "" {
			method = attributeText(attributes, "http.method")
		}
		return joinNonEmpty(" ", method, route)
	}
	if method := attributeText(attributes, "rpc.method"); method != "" {
		return joinNonEmpty("/", attributeText(attributes, "rpc.service"), method)
	}
	if name != "" {
		return name
	}
	return "(unnamed)"
}

func SpanRequestFailed(status string, attributes map[string]any) bool {
	if status == "ERROR" {
		return true
	}
	code := attributeText(attributes, "http.response.status_code")
	if code == "" {
		code = attributeText(attributes, "http.status_code")
	}
	return len(code) == 3 && code[0] == '5' && isDigit(code[1]) && isDigit(code[2])
}

func SpanDependency(attributes map[string]any, kind int32) (dependencyType, dependencyName string) {
	database := hasAttribute(attributes, "db.system") || hasAttribute(attributes, "db.system.name")
	messaging := hasAttribute(attributes, "messaging.system")
	switch {
	case database:
		return "database", firstAttributeText(attributes, "db.namespace", "db.name", "server.address", "db.system.name", "db.system")
	case messaging:
		return "queue", firstAttributeText(attributes, "messaging.destination.name", "messaging.destination", "messaging.system")
	case kind == spanKindClient:
		return "external", firstAttributeText(attributes, "peer.service", "server.address", "http.host", "url.full")
	default:
		return "", ""
	}
}

func hasAttribute(attributes map[string]any, key string) bool {
	_, found := attributes[key]
	return found
}

// attributeText reads a value as text: strings as is, others as JSON.
func attributeText(attributes map[string]any, key string) string {
	value, found := attributes[key]
	if !found || value == nil {
		return ""
	}
	if text, isText := value.(string); isText {
		return text
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func firstAttributeText(attributes map[string]any, keys ...string) string {
	for _, key := range keys {
		if text := attributeText(attributes, key); text != "" {
			return text
		}
	}
	return ""
}

func joinNonEmpty(separator string, values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, separator)
}

func isDigit(character byte) bool { return character >= '0' && character <= '9' }
