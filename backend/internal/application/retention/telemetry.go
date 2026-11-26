package retention

import (
	"context"
	"time"
)

// policyLocks keeps retention changes serialized with the cleanup they govern.
type policyLocks interface {
	WithLongestRetention(context.Context, time.Time, time.Duration, func(time.Time) error) error
	WithProjectRetention(context.Context, string, time.Time, time.Duration, func(time.Time) error) error
}

type telemetryExpirer interface {
	DropPartitionsBefore(context.Context, time.Time) error
	ExpireProjectTelemetry(context.Context, string, time.Time) error
}

type TelemetryMaintenance struct {
	policies  policyLocks
	telemetry telemetryExpirer
}

func NewTelemetryMaintenance(policies policyLocks, telemetry telemetryExpirer) *TelemetryMaintenance {
	return &TelemetryMaintenance{policies: policies, telemetry: telemetry}
}

func (maintenance *TelemetryMaintenance) DropExpiredPartitions(ctx context.Context, now time.Time, fallback time.Duration) error {
	return maintenance.policies.WithLongestRetention(ctx, now, fallback, func(before time.Time) error {
		return maintenance.telemetry.DropPartitionsBefore(ctx, before)
	})
}

func (maintenance *TelemetryMaintenance) PruneProject(ctx context.Context, project string, now time.Time, fallback time.Duration) error {
	return maintenance.policies.WithProjectRetention(ctx, project, now, fallback, func(before time.Time) error {
		return maintenance.telemetry.ExpireProjectTelemetry(ctx, project, before)
	})
}
