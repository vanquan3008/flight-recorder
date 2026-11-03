package pgvalue

import (
	"math"
	"time"
)

// OrNil returns nil for the zero value, so an optional field is stored as NULL
// through a sqlc parameter.
func OrNil[T comparable](value T) *T {
	var zero T
	if value == zero {
		return nil
	}
	return &value
}

// TimeOrNil returns nil for the zero time.
func TimeOrNil(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

// NanosOrNil stores a duration as nanoseconds, or NULL when it is zero.
func NanosOrNil(value time.Duration) *int64 {
	return OrNil(value.Nanoseconds())
}

// StringOrEmpty reads a nullable text column as "" when it is NULL.
func StringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// IntPointer widens a nullable PostgreSQL integer to the int domain models use.
func IntPointer(value *int32) *int {
	if value == nil {
		return nil
	}
	widened := int(*value)
	return &widened
}

// ClampInt32 stores an OTLP uint32 counter in a PostgreSQL integer column,
// saturating instead of wrapping to a negative value.
func ClampInt32(value uint32) int32 {
	return int32(min(value, math.MaxInt32))
}

// FiniteOrNil stores NaN and ±Inf as NULL; PostgreSQL aggregates over them
// would otherwise poison every result they touch.
func FiniteOrNil(value *float64) *float64 {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return nil
	}
	return value
}

// BytesOrNil stores an empty document as NULL.
func BytesOrNil(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	return value
}
