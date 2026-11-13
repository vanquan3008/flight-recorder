// Package pgvalue converts domain values to PostgreSQL parameters and
// classifies PostgreSQL errors. Every feature store shares it.
package pgvalue

import "encoding/json"

// MarshalJSON encodes a JSON document; nil becomes JSON null.
func MarshalJSON(value any) ([]byte, error) {
	if value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(value)
}

// MarshalJSONObject encodes an object column; nil becomes {}.
func MarshalJSONObject(value map[string]any) ([]byte, error) {
	if value == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(value)
}

// MarshalJSONArray encodes an array column; nil becomes [].
func MarshalJSONArray[T any](value []T) ([]byte, error) {
	if value == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(value)
}

// EntityID names the entity an evidence row refers to, such as "service:checkout".
func EntityID(kind, name string) string { return kind + ":" + name }
