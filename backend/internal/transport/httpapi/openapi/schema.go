package openapi

import (
	"encoding"
	"encoding/json"
	"reflect"
	"strings"
	"time"
	"unicode"
)

var (
	timeType          = reflect.TypeFor[time.Time]()
	durationType      = reflect.TypeFor[time.Duration]()
	rawMessageType    = reflect.TypeFor[json.RawMessage]()
	jsonMarshalerType = reflect.TypeFor[json.Marshaler]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
)

// schemas turns Go types into JSON Schema the way encoding/json writes them.
// Named structs become components so generators produce one class per type.
type schemas struct {
	components map[string]any
	names      map[reflect.Type]string
	owners     map[string]reflect.Type
}

func newSchemas() *schemas {
	return &schemas{components: map[string]any{}, names: map[reflect.Type]string{}, owners: map[string]reflect.Type{}}
}

func ref(group, name string) map[string]any {
	return map[string]any{"$ref": "#/components/" + group + "/" + name}
}

// add registers a hand-written component schema.
func (registry *schemas) add(name string, schema map[string]any) map[string]any {
	registry.components[name] = schema
	if _, owned := registry.owners[name]; !owned {
		// A nil owner keeps Go types from claiming a hand-written name.
		registry.owners[name] = nil
	}
	return ref("schemas", name)
}

// schemaFor returns the schema of a value of type t, registering named structs.
func (registry *schemas) schemaFor(t reflect.Type) map[string]any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case t == timeType:
		return map[string]any{"type": "string", "format": "date-time"}
	case t == durationType:
		return map[string]any{"type": "integer", "format": "int64", "description": "Duration in nanoseconds."}
	case t == rawMessageType:
		return map[string]any{}
	case t.Implements(jsonMarshalerType) || reflect.PointerTo(t).Implements(jsonMarshalerType):
		return map[string]any{}
	case t.Kind() != reflect.String && (t.Implements(textMarshalerType) || reflect.PointerTo(t).Implements(textMarshalerType)):
		return map[string]any{"type": "string"}
	}
	switch t.Kind() {
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int8, reflect.Int16, reflect.Int32:
		return map[string]any{"type": "integer", "format": "int32"}
	case reflect.Int, reflect.Int64:
		return map[string]any{"type": "integer", "format": "int64"}
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint, reflect.Uint64, reflect.Uintptr:
		return map[string]any{"type": "integer", "minimum": 0}
	case reflect.Float32:
		return map[string]any{"type": "number", "format": "float"}
	case reflect.Float64:
		return map[string]any{"type": "number", "format": "double"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return map[string]any{"type": "string", "format": "byte"}
		}
		return map[string]any{"type": "array", "items": registry.schemaFor(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": registry.schemaFor(t.Elem())}
	case reflect.Struct:
		if t.Name() == "" {
			return registry.structSchema(t)
		}
		return registry.named(t)
	default:
		// Interfaces and anything encoding/json cannot describe statically accept any value.
		return map[string]any{}
	}
}

// named registers a named struct once and returns a reference to it.
func (registry *schemas) named(t reflect.Type) map[string]any {
	if name, ok := registry.names[t]; ok {
		return ref("schemas", name)
	}
	name := registry.componentName(t)
	registry.names[t] = name
	registry.owners[name] = t
	// Register before building so a recursive type refers to itself.
	registry.components[name] = map[string]any{}
	registry.components[name] = registry.structSchema(t)
	return ref("schemas", name)
}

// componentName is the exported type name, qualified by its package when two packages share it.
func (registry *schemas) componentName(t reflect.Type) string {
	name := sanitizeName(exported(t.Name()))
	if owner, taken := registry.owners[name]; taken && owner != t {
		pkg := t.PkgPath()
		name = sanitizeName(exported(pkg[strings.LastIndex(pkg, "/")+1:]) + exported(t.Name()))
	}
	return name
}

// envelope registers a named wrapper such as LogRecordPage around item schemas.
func (registry *schemas) envelope(item reflect.Type, suffix string, paginated bool) map[string]any {
	for item.Kind() == reflect.Pointer {
		item = item.Elem()
	}
	base := "Item"
	if item.Name() != "" {
		registry.schemaFor(item)
		base = registry.names[item]
		if base == "" {
			base = sanitizeName(exported(item.Name()))
		}
	}
	name := base + suffix
	if _, exists := registry.components[name]; exists {
		return ref("schemas", name)
	}
	properties := map[string]any{"data": map[string]any{"type": "array", "items": registry.schemaFor(item)}}
	if paginated {
		properties["next_cursor"] = map[string]any{"type": "string", "description": "Opaque cursor for the next page; absent on the last page."}
	}
	return registry.add(name, map[string]any{"type": "object", "required": []string{"data"}, "properties": properties})
}

// structSchema lists the fields encoding/json writes, flattening embedded structs.
func (registry *schemas) structSchema(t reflect.Type) map[string]any {
	properties := map[string]any{}
	required := []string{}
	registry.collectFields(t, properties, &required)
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func (registry *schemas) collectFields(t reflect.Type, properties map[string]any, required *[]string) {
	for index := range t.NumField() {
		field := t.Field(index)
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, options, _ := strings.Cut(tag, ",")
		fieldType := field.Type
		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		if field.Anonymous && name == "" && fieldType.Kind() == reflect.Struct {
			registry.collectFields(fieldType, properties, required)
			continue
		}
		if !field.IsExported() {
			continue
		}
		if name == "" {
			name = field.Name
		}
		var schema map[string]any
		if hasOption(options, "string") {
			schema = map[string]any{"type": "string"}
		} else {
			schema = registry.schemaFor(field.Type)
		}
		optional := hasOption(options, "omitempty") || hasOption(options, "omitzero")
		if field.Type.Kind() == reflect.Pointer && !optional {
			schema = nullable(schema)
		}
		properties[name] = schema
		if !optional && field.Type.Kind() != reflect.Pointer {
			*required = append(*required, name)
		}
	}
}

// nullable marks a schema that encoding/json writes as null for a nil pointer.
func nullable(schema map[string]any) map[string]any {
	if kind, ok := schema["type"].(string); ok {
		copied := make(map[string]any, len(schema))
		for key, value := range schema {
			copied[key] = value
		}
		copied["type"] = []string{kind, "null"}
		return copied
	}
	return map[string]any{"anyOf": []any{schema, map[string]any{"type": "null"}}}
}

func hasOption(options, want string) bool {
	for option := range strings.SplitSeq(options, ",") {
		if option == want {
			return true
		}
	}
	return false
}

func exported(name string) string {
	if name == "" {
		return name
	}
	runes := []rune(name)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// sanitizeName keeps the characters OpenAPI allows in component names.
func sanitizeName(name string) string {
	return strings.Map(func(r rune) rune {
		if r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_') {
			return r
		}
		return '_'
	}, name)
}
