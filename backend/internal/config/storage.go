package config

import (
	"time"
)

// StorageConfig aggregates storage subsystems including partition retention policies.
type StorageConfig struct {
	Retention RetentionConfig `config:"retention"`
}

// RetentionConfig controls telemetry, incident, audit, and delivery data expiration.
// A zero MaxAge keeps the corresponding data indefinitely.
type RetentionConfig struct {
	MaxAge         time.Duration `config:"max_age"`
	IncidentMaxAge time.Duration `config:"incident_max_age"`
	AuditMaxAge    time.Duration `config:"audit_max_age"`
	DeliveryMaxAge time.Duration `config:"delivery_max_age"`
	CheckInterval  time.Duration `config:"check_interval"`
	BatchSize      int           `config:"batch_size"`
}

func (configuration Config) validateStorage(v *validator) {
	configuration.validateRetentionLimits(v)
}

func (configuration Config) validateRetentionLimits(v *validator) {
	if configuration.Retention.MaxAge < 0 {
		v.add("retention.max_age", "must not be negative")
	}
	if configuration.Retention.IncidentMaxAge < 0 {
		v.add("retention.incident_max_age", "must not be negative")
	}
	if configuration.Retention.AuditMaxAge < 0 {
		v.add("retention.audit_max_age", "must not be negative")
	}
	if configuration.Retention.DeliveryMaxAge < 0 {
		v.add("retention.delivery_max_age", "must not be negative")
	}
	if configuration.Retention.CheckInterval <= 0 {
		v.add("retention.check_interval", "must be positive")
	}
	if configuration.Retention.BatchSize <= 0 || configuration.Retention.BatchSize > maxIngestionBatchSize {
		v.add("retention.batch_size", "must be between 1 and 10000")
	}
}
