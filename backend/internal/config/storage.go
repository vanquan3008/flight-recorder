package config

import "time"

type StorageConfig struct {
	Retention RetentionConfig `config:"retention"`
}

type RetentionConfig struct {
	MaxAge         time.Duration `config:"max_age"`
	IncidentMaxAge time.Duration `config:"incident_max_age"`
	AuditMaxAge    time.Duration `config:"audit_max_age"`
	DeliveryMaxAge time.Duration `config:"delivery_max_age"`
	CheckInterval  time.Duration `config:"check_interval"`
	BatchSize      int           `config:"batch_size"`
}

func (configuration Config) validateStorage(validator *validator) {
	retention := configuration.Storage.Retention

	if retention.MaxAge < 0 {
		validator.add("storage.retention.max_age", "must not be negative")
	}
	if retention.IncidentMaxAge < 0 {
		validator.add("storage.retention.incident_max_age", "must not be negative")
	}
	if retention.AuditMaxAge < 0 {
		validator.add("storage.retention.audit_max_age", "must not be negative")
	}
	if retention.DeliveryMaxAge < 0 {
		validator.add("storage.retention.delivery_max_age", "must not be negative")
	}
	if retention.CheckInterval <= 0 {
		validator.add("storage.retention.check_interval", "must be positive")
	}
	if retention.BatchSize <= 0 || retention.BatchSize > maxBatchSize {
		validator.add("storage.retention.batch_size", "must be between 1 and 10000")
	}
}
