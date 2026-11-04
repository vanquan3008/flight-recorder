package config

import (
	"regexp"
	"strings"
	"time"
)

const (
	maxIngestionRecords            = 100_000
	maxIngestionAttributes         = 1_024
	maxIngestionAttributeBytes     = 1 << 20 // 1 MiB
	maxIngestionQueueSize          = 100_000
	maxIngestionConcurrentRequests = 1_024
	maxIngestionQueueBytes         = 4 << 30 // 4 GiB
	maxIngestionBatchSize          = 10_000
	maxIngestionWorkers            = 64
)

var projectSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// QueueConfig controls asynchronous batch processing and buffer limits for telemetry and workers.
type QueueConfig struct {
	QueueSize      int           `config:"queue_size"`
	QueueMaxBytes  int64         `config:"queue_max_bytes"`
	EnqueueTimeout time.Duration `config:"enqueue_timeout"`
	BatchSize      int           `config:"batch_size"`
	BatchTimeout   time.Duration `config:"batch_timeout"`
	FlushInterval  time.Duration `config:"flush_interval"`
	Workers        int           `config:"workers"`
}

// IngestionConfig defines OTLP reception, ingestion queue bounds, and tenant scoping.
type IngestionConfig struct {
	GRPCAddress            string        `config:"grpc_address"`
	HTTPAddress            string        `config:"http_address"`
	ProjectID              string        `config:"project_id"`
	ProjectSlug            string        `config:"project_slug"`
	ProjectName            string        `config:"project_name"`
	EnvironmentID          string        `config:"environment_id"`
	EnvironmentName        string        `config:"environment_name"`
	MaxRequestBytes        int64         `config:"max_request_bytes"`
	MaxRecordsPerRequest   int           `config:"max_records_per_request"`
	MaxConcurrentRequests  int           `config:"max_concurrent_requests"`
	MaxAttributesPerRecord int           `config:"max_attributes_per_record"`
	MaxAttributeValueBytes int           `config:"max_attribute_value_bytes"`
	MaxAttributeDepth      int           `config:"max_attribute_depth"`
	QueueSize              int           `config:"queue_size"`
	QueueMaxBytes          int64         `config:"queue_max_bytes"`
	EnqueueTimeout         time.Duration `config:"enqueue_timeout"`
	BatchSize              int           `config:"batch_size"`
	BatchTimeout           time.Duration `config:"batch_timeout"`
	FlushInterval          time.Duration `config:"flush_interval"`
	Workers                int           `config:"workers"`
	RedactedKeys           []string      `config:"redacted_keys"`

	// Limits failed authentication attempts per client address.
	// Zero disables this protection.
	AuthFailureLimit  int           `config:"auth_failure_limit"`
	AuthFailureWindow time.Duration `config:"auth_failure_window"`

	// Scrubs credentials from free text, URLs and sensitive attributes.
	ScrubFreeText bool `config:"scrub_free_text"`
}

func (configuration Config) validateQueue(v *validator) {
	configuration.validateIngestion(v)
}

func (configuration Config) validateIngestion(v *validator) {
	if strings.TrimSpace(configuration.Ingestion.GRPCAddress) == "" {
		v.add("ingestion.grpc_address", "must not be empty")
	}
	if strings.TrimSpace(configuration.Ingestion.HTTPAddress) == "" {
		v.add("ingestion.http_address", "must not be empty")
	}
	if configuration.HTTP.Address == configuration.Ingestion.GRPCAddress {
		v.add("ingestion.grpc_address", "management and OTLP/gRPC addresses must be distinct")
	}
	if configuration.HTTP.Address == configuration.Ingestion.HTTPAddress {
		v.add("ingestion.http_address", "management and OTLP/HTTP addresses must be distinct")
	}
	if configuration.Ingestion.GRPCAddress == configuration.Ingestion.HTTPAddress {
		v.add("ingestion.grpc_address", "OTLP/gRPC and OTLP/HTTP addresses must be distinct")
	}
	if !validUUID(configuration.Ingestion.ProjectID) {
		v.add("ingestion.project_id", "must be a valid UUID")
	}
	if !validUUID(configuration.Ingestion.EnvironmentID) {
		v.add("ingestion.environment_id", "must be a valid UUID")
	}
	if !projectSlugPattern.MatchString(configuration.Ingestion.ProjectSlug) {
		v.add("ingestion.project_slug", "has an invalid format (lowercase letters, digits, hyphens; must start with letter or digit)")
	}
	if strings.TrimSpace(configuration.Ingestion.ProjectName) == "" {
		v.add("ingestion.project_name", "must not be empty")
	}
	if strings.TrimSpace(configuration.Ingestion.EnvironmentName) == "" {
		v.add("ingestion.environment_name", "must not be empty")
	}
	configuration.validateIngestionLimits(v)
	configuration.validateIngestionQueue(v)
}

func (configuration Config) validateIngestionLimits(v *validator) {
	if configuration.Ingestion.MaxRequestBytes <= 0 || configuration.Ingestion.MaxRequestBytes > 64<<20 {
		v.add("ingestion.max_request_bytes", "must be between 1 and 67108864")
	}
	if configuration.Ingestion.MaxRecordsPerRequest <= 0 || configuration.Ingestion.MaxRecordsPerRequest > maxIngestionRecords {
		v.add("ingestion.max_records_per_request", "must be between 1 and 100000")
	}
	if configuration.Ingestion.MaxConcurrentRequests <= 0 || configuration.Ingestion.MaxConcurrentRequests > maxIngestionConcurrentRequests {
		v.add("ingestion.max_concurrent_requests", "must be between 1 and 1024")
	}
	if configuration.Ingestion.MaxAttributesPerRecord <= 0 || configuration.Ingestion.MaxAttributesPerRecord > maxIngestionAttributes {
		v.add("ingestion.max_attributes_per_record", "must be between 1 and 1024")
	}
	if configuration.Ingestion.MaxAttributeValueBytes <= 0 || configuration.Ingestion.MaxAttributeValueBytes > maxIngestionAttributeBytes {
		v.add("ingestion.max_attribute_value_bytes", "must be between 1 and 1048576")
	}
	if configuration.Ingestion.MaxAttributeDepth <= 0 || configuration.Ingestion.MaxAttributeDepth > 16 {
		v.add("ingestion.max_attribute_depth", "must be between 1 and 16")
	}
	if configuration.Ingestion.AuthFailureLimit < 0 {
		v.add("ingestion.auth_failure_limit", "must not be negative (0 disables it)")
	}
	if configuration.Ingestion.AuthFailureLimit > 0 && configuration.Ingestion.AuthFailureWindow <= 0 {
		v.add("ingestion.auth_failure_window", "must be positive when auth_failure_limit is set")
	}
}

func (configuration Config) validateIngestionQueue(v *validator) {
	if configuration.Ingestion.BatchSize <= 0 || configuration.Ingestion.BatchSize > maxIngestionBatchSize {
		v.add("ingestion.batch_size", "must be between 1 and 10000")
	}
	if configuration.Ingestion.QueueSize < configuration.Ingestion.BatchSize {
		v.add("ingestion.queue_size", "must be >= batch_size")
	}
	if configuration.Ingestion.QueueSize > maxIngestionQueueSize {
		v.add("ingestion.queue_size", "must not exceed 100000")
	}
	if configuration.Ingestion.QueueMaxBytes < configuration.Ingestion.MaxRequestBytes {
		v.add("ingestion.queue_max_bytes", "must be >= max_request_bytes")
	}
	if configuration.Ingestion.QueueMaxBytes > maxIngestionQueueBytes {
		v.add("ingestion.queue_max_bytes", "must not exceed 4294967296")
	}
	if configuration.Ingestion.Workers <= 0 || configuration.Ingestion.Workers > maxIngestionWorkers {
		v.add("ingestion.workers", "must be between 1 and 64")
	}
	if configuration.Ingestion.EnqueueTimeout <= 0 {
		v.add("ingestion.enqueue_timeout", "must be positive")
	}
	if configuration.Ingestion.BatchTimeout <= 0 {
		v.add("ingestion.batch_timeout", "must be positive")
	}
	if configuration.Ingestion.FlushInterval <= 0 {
		v.add("ingestion.flush_interval", "must be positive")
	}
}
