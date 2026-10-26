package otlp

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const maxJSONDepth = 64

// OTLP JSON uses hex for correlation IDs and base64 for ordinary bytes.
// Keep Protobuf's type validation after adapting those protocol differences.
func unmarshalOTLPJSON(body []byte, message proto.Message) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var fields map[string]any
	if err := decoder.Decode(&fields); err != nil {
		return err
	}
	if len(bytes.TrimSpace(body[decoder.InputOffset():])) != 0 {
		return errors.New("trailing OTLP JSON")
	}
	if err := adaptJSONFields(fields, message.ProtoReflect().Descriptor(), 0); err != nil {
		return err
	}
	adapted, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	return (protojson.UnmarshalOptions{DiscardUnknown: true, RecursionLimit: maxJSONDepth}).Unmarshal(adapted, message)
}

func adaptJSONFields(fields map[string]any, descriptor protoreflect.MessageDescriptor, depth int) error {
	if depth > maxJSONDepth {
		return errors.New("OTLP JSON nesting exceeds limit")
	}
	for name, value := range fields {
		field := descriptor.Fields().ByJSONName(name)
		if field == nil {
			delete(fields, name)
			continue
		}
		if value == nil {
			continue
		}
		if field.Kind() == protoreflect.EnumKind {
			if _, numeric := value.(json.Number); !numeric {
				return errors.New("OTLP JSON enums must be numbers")
			}
		}
		if field.Kind() == protoreflect.BytesKind && (name == "traceId" || name == "spanId" || name == "parentSpanId") {
			text, ok := value.(string)
			if !ok {
				return errors.New("OTLP JSON ID must be a hex string")
			}
			decoded, err := hex.DecodeString(text)
			if err != nil {
				return err
			}
			fields[name] = base64.StdEncoding.EncodeToString(decoded)
		}
		if field.Kind() != protoreflect.MessageKind {
			continue
		}
		values := []any{value}
		if field.IsList() {
			values, _ = value.([]any)
		}
		for _, nested := range values {
			if object, ok := nested.(map[string]any); ok {
				if err := adaptJSONFields(object, field.Message(), depth+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
