package config

import (
	"regexp"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

const (
	maxIngestionRequestBytes       = 64 << 20
	maxIngestionRecords            = 100_000
	maxIngestionConcurrentRequests = 1_024
	maxIngestionAttributes         = 1_024
	maxIngestionAttributeBytes     = 1 << 20
	maxIngestionAttributeDepth     = 16
)

var projectSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

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
	RedactedKeys           []string      `config:"redacted_keys"`
	AuthFailureLimit       int           `config:"auth_failure_limit"`
	AuthFailureWindow      time.Duration `config:"auth_failure_window"`
	ScrubFreeText          bool          `config:"scrub_free_text"`
}

func (configuration Config) validateIngestion(validator *validator) {
	configuration.validateIngestionAddresses(validator)
	configuration.Ingestion.validateScope(validator)
	configuration.Ingestion.validateLimits(validator)
}

func (configuration Config) validateIngestionAddresses(validator *validator) {
	ingestion := configuration.Ingestion

	if strings.TrimSpace(ingestion.GRPCAddress) == "" {
		validator.add("ingestion.grpc_address", "must not be empty")
	}
	if strings.TrimSpace(ingestion.HTTPAddress) == "" {
		validator.add("ingestion.http_address", "must not be empty")
	}
	if configuration.HTTP.Address == ingestion.GRPCAddress {
		validator.add("ingestion.grpc_address", "management and OTLP/gRPC addresses must be distinct")
	}
	if configuration.HTTP.Address == ingestion.HTTPAddress {
		validator.add("ingestion.http_address", "management and OTLP/HTTP addresses must be distinct")
	}
	if ingestion.GRPCAddress == ingestion.HTTPAddress {
		validator.add("ingestion.grpc_address", "OTLP/gRPC and OTLP/HTTP addresses must be distinct")
	}
}

func (ingestion IngestionConfig) validateScope(validator *validator) {
	if !validID(ingestion.ProjectID) {
		validator.add("ingestion.project_id", "must be a valid UUID")
	}
	if !validID(ingestion.EnvironmentID) {
		validator.add("ingestion.environment_id", "must be a valid UUID")
	}
	if !projectSlugPattern.MatchString(ingestion.ProjectSlug) {
		validator.add("ingestion.project_slug", "has an invalid format (lowercase letters, digits, hyphens; must start with letter or digit)")
	}
	if strings.TrimSpace(ingestion.ProjectName) == "" {
		validator.add("ingestion.project_name", "must not be empty")
	}
	if strings.TrimSpace(ingestion.EnvironmentName) == "" {
		validator.add("ingestion.environment_name", "must not be empty")
	}
}

func (ingestion IngestionConfig) validateLimits(validator *validator) {
	if ingestion.MaxRequestBytes <= 0 || ingestion.MaxRequestBytes > maxIngestionRequestBytes {
		validator.add("ingestion.max_request_bytes", "must be between 1 and 67108864")
	}
	if ingestion.MaxRecordsPerRequest <= 0 || ingestion.MaxRecordsPerRequest > maxIngestionRecords {
		validator.add("ingestion.max_records_per_request", "must be between 1 and 100000")
	}
	if ingestion.MaxConcurrentRequests <= 0 || ingestion.MaxConcurrentRequests > maxIngestionConcurrentRequests {
		validator.add("ingestion.max_concurrent_requests", "must be between 1 and 1024")
	}
	if ingestion.MaxAttributesPerRecord <= 0 || ingestion.MaxAttributesPerRecord > maxIngestionAttributes {
		validator.add("ingestion.max_attributes_per_record", "must be between 1 and 1024")
	}
	if ingestion.MaxAttributeValueBytes <= 0 || ingestion.MaxAttributeValueBytes > maxIngestionAttributeBytes {
		validator.add("ingestion.max_attribute_value_bytes", "must be between 1 and 1048576")
	}
	if ingestion.MaxAttributeDepth <= 0 || ingestion.MaxAttributeDepth > maxIngestionAttributeDepth {
		validator.add("ingestion.max_attribute_depth", "must be between 1 and 16")
	}
	if ingestion.AuthFailureLimit < 0 {
		validator.add("ingestion.auth_failure_limit", "must not be negative (0 disables it)")
	}
	if ingestion.AuthFailureLimit > 0 && ingestion.AuthFailureWindow <= 0 {
		validator.add("ingestion.auth_failure_window", "must be positive when auth_failure_limit is set")
	}
}

// validID accepts a UUID that is not the all-zero one, which stands for "unset".
func validID(value string) bool {
	return telemetry.ValidUUID(value) && strings.Trim(value, "0-") != ""
}
