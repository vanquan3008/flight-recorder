package config

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var projectSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func (configuration Config) validateIngestion() error {
	if strings.TrimSpace(configuration.Ingestion.GRPCAddress) == "" {
		return errors.New("ingestion.grpc_address must not be empty")
	}
	if strings.TrimSpace(configuration.Ingestion.HTTPAddress) == "" {
		return errors.New("ingestion.http_address must not be empty")
	}
	if configuration.HTTP.Address == configuration.Ingestion.GRPCAddress {
		return errors.New("management and OTLP/gRPC addresses must be distinct")
	}
	if configuration.HTTP.Address == configuration.Ingestion.HTTPAddress {
		return errors.New("management and OTLP/HTTP addresses must be distinct")
	}
	if configuration.Ingestion.GRPCAddress == configuration.Ingestion.HTTPAddress {
		return errors.New("OTLP/gRPC and OTLP/HTTP addresses must be distinct")
	}
	if !validUUID(configuration.Ingestion.ProjectID) {
		return errors.New("ingestion.project_id must be a valid UUID")
	}
	if !validUUID(configuration.Ingestion.EnvironmentID) {
		return errors.New("ingestion.environment_id must be a valid UUID")
	}
	if !projectSlugPattern.MatchString(configuration.Ingestion.ProjectSlug) {
		return errors.New("ingestion.project_slug has an invalid format (lowercase letters, digits, hyphens; must start with letter or digit)")
	}
	if strings.TrimSpace(configuration.Ingestion.ProjectName) == "" {
		return errors.New("ingestion.project_name must not be empty")
	}
	if strings.TrimSpace(configuration.Ingestion.EnvironmentName) == "" {
		return errors.New("ingestion.environment_name must not be empty")
	}
	if err := configuration.validateIngestionLimits(); err != nil {
		return err
	}
	return configuration.validateIngestionQueue()
}

func (configuration Config) validateIngestionLimits() error {
	if configuration.Ingestion.MaxRequestBytes <= 0 || configuration.Ingestion.MaxRequestBytes > 64<<20 {
		return fmt.Errorf("ingestion.max_request_bytes must be between 1 and %d", 64<<20)
	}
	if configuration.Ingestion.MaxRecordsPerRequest <= 0 || configuration.Ingestion.MaxRecordsPerRequest > maxIngestionRecords {
		return fmt.Errorf("ingestion.max_records_per_request must be between 1 and %d", maxIngestionRecords)
	}
	if configuration.Ingestion.MaxAttributesPerRecord <= 0 || configuration.Ingestion.MaxAttributesPerRecord > maxIngestionAttributes {
		return fmt.Errorf("ingestion.max_attributes_per_record must be between 1 and %d", maxIngestionAttributes)
	}
	if configuration.Ingestion.MaxAttributeValueBytes <= 0 || configuration.Ingestion.MaxAttributeValueBytes > maxIngestionAttributeBytes {
		return fmt.Errorf("ingestion.max_attribute_value_bytes must be between 1 and %d", maxIngestionAttributeBytes)
	}
	if configuration.Ingestion.MaxAttributeDepth <= 0 || configuration.Ingestion.MaxAttributeDepth > 16 {
		return errors.New("ingestion.max_attribute_depth must be between 1 and 16")
	}
	return nil
}

func (configuration Config) validateIngestionQueue() error {
	if configuration.Ingestion.BatchSize <= 0 || configuration.Ingestion.BatchSize > maxIngestionBatchSize {
		return fmt.Errorf("ingestion.batch_size must be between 1 and %d", maxIngestionBatchSize)
	}
	if configuration.Ingestion.QueueSize < configuration.Ingestion.BatchSize {
		return errors.New("ingestion.queue_size must be >= batch_size")
	}
	if configuration.Ingestion.QueueSize > maxIngestionQueueSize {
		return fmt.Errorf("ingestion.queue_size must not exceed %d", maxIngestionQueueSize)
	}
	if configuration.Ingestion.QueueMaxBytes < configuration.Ingestion.MaxRequestBytes {
		return errors.New("ingestion.queue_max_bytes must be >= max_request_bytes")
	}
	if configuration.Ingestion.QueueMaxBytes > maxIngestionQueueBytes {
		return fmt.Errorf("ingestion.queue_max_bytes must not exceed %d", maxIngestionQueueBytes)
	}
	if configuration.Ingestion.Workers <= 0 || configuration.Ingestion.Workers > maxIngestionWorkers {
		return fmt.Errorf("ingestion.workers must be between 1 and %d", maxIngestionWorkers)
	}
	if configuration.Ingestion.EnqueueTimeout <= 0 {
		return errors.New("ingestion.enqueue_timeout must be positive")
	}
	if configuration.Ingestion.BatchTimeout <= 0 {
		return errors.New("ingestion.batch_timeout must be positive")
	}
	if configuration.Ingestion.FlushInterval <= 0 {
		return errors.New("ingestion.flush_interval must be positive")
	}
	return nil
}
