package telemetry

import (
	"math"
	"time"
)

const MaxRetentionDays = math.MaxInt64 / int64(24*time.Hour)

func RetentionAge(days *int32, fallback time.Duration) time.Duration {
	if days == nil {
		return fallback
	}
	return time.Duration(*days) * 24 * time.Hour
}
