package otlp

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"strings"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
)

// attributeBuilder enforces the ingestion limits. The first value for a key
// wins, and any violation rejects the whole record.
type attributeBuilder struct {
	normalizer *normalizer
	values     map[string]any
	count      int
}

func newAttributeBuilder(normalizer *normalizer) *attributeBuilder {
	return &attributeBuilder{normalizer: normalizer, values: make(map[string]any)}
}

func (builder *attributeBuilder) addAll(attributes []*commonpb.KeyValue) error {
	for _, attribute := range attributes {
		key := strings.TrimSpace(attribute.GetKey())
		if addError := builder.add(key, attribute.GetValue()); addError != nil {
			return addError
		}
	}
	return nil
}

func (builder *attributeBuilder) add(key string, value *commonpb.AnyValue) error {
	if builder.normalizer.redacts(key) {
		return builder.addSynthetic(key, "[REDACTED]")
	}
	converted, convertError := builder.value(value, 0)
	if convertError != nil {
		return convertError
	}
	return builder.addSynthetic(key, converted)
}

func (builder *attributeBuilder) addSynthetic(key string, value any) error {
	builder.count++
	if builder.count > builder.normalizer.configuration.MaxAttributesPerRecord {
		return errors.New("attribute count exceeds limit")
	}
	if key == "" || len(key) > builder.normalizer.configuration.MaxAttributeValueBytes {
		return errors.New("attribute key is empty or too long")
	}
	if containsNUL(key) {
		return errNULText
	}
	if builder.normalizer.redacts(key) {
		value = "[REDACTED]"
	}
	encodedValue, encodeError := json.Marshal(value)
	if encodeError != nil || len(encodedValue) > builder.normalizer.configuration.MaxAttributeValueBytes {
		return errors.New("encoded attribute value exceeds limit")
	}
	if _, exists := builder.values[key]; !exists {
		builder.values[key] = value
	}
	return nil
}

func (builder *attributeBuilder) value(value *commonpb.AnyValue, depth int) (any, error) {
	if value == nil {
		return nil, nil
	}
	if depth > builder.normalizer.configuration.MaxAttributeDepth {
		return nil, errors.New("attribute nesting exceeds limit")
	}
	switch typedValue := value.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		if len(typedValue.StringValue) > builder.normalizer.configuration.MaxAttributeValueBytes {
			return nil, errors.New("attribute string exceeds limit")
		}
		if containsNUL(typedValue.StringValue) {
			return nil, errNULText
		}
		return builder.normalizer.scrub(typedValue.StringValue), nil
	case *commonpb.AnyValue_BoolValue:
		return typedValue.BoolValue, nil
	case *commonpb.AnyValue_IntValue:
		return typedValue.IntValue, nil
	case *commonpb.AnyValue_DoubleValue:
		if math.IsNaN(typedValue.DoubleValue) || math.IsInf(typedValue.DoubleValue, 0) {
			return nil, errors.New("attribute number is not finite")
		}
		return typedValue.DoubleValue, nil
	case *commonpb.AnyValue_BytesValue:
		if len(typedValue.BytesValue) > builder.normalizer.configuration.MaxAttributeValueBytes {
			return nil, errors.New("attribute bytes exceed limit")
		}
		return base64.StdEncoding.EncodeToString(typedValue.BytesValue), nil
	case *commonpb.AnyValue_ArrayValue:
		values := typedValue.ArrayValue.GetValues()
		if len(values) > builder.normalizer.configuration.MaxAttributesPerRecord {
			return nil, errors.New("attribute array exceeds limit")
		}
		result := make([]any, 0, len(values))
		for _, nested := range values {
			converted, convertError := builder.value(nested, depth+1)
			if convertError != nil {
				return nil, convertError
			}
			result = append(result, converted)
		}
		return result, nil
	case *commonpb.AnyValue_KvlistValue:
		result := make(map[string]any)
		for _, nested := range typedValue.KvlistValue.GetValues() {
			builder.count++
			if builder.count > builder.normalizer.configuration.MaxAttributesPerRecord {
				return nil, errors.New("attribute count exceeds limit")
			}
			key := strings.TrimSpace(nested.GetKey())
			if key == "" || len(key) > builder.normalizer.configuration.MaxAttributeValueBytes {
				return nil, errors.New("attribute key is empty or too long")
			}
			if containsNUL(key) {
				return nil, errNULText
			}
			if builder.normalizer.redacts(key) {
				result[key] = "[REDACTED]"
				continue
			}
			converted, convertError := builder.value(nested.GetValue(), depth+1)
			if convertError != nil {
				return nil, convertError
			}
			if _, exists := result[key]; !exists {
				result[key] = converted
			}
		}
		return result, nil
	default:
		return nil, nil
	}
}
