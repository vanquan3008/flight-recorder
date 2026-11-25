package pgvalue

import "time"

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
