package config

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var projectSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func (configuration Config) Validate() error {
	if configuration.Version != CurrentVersion {
		return fmt.Errorf("configuration version must be %d", CurrentVersion)
	}
	if validationError := configuration.validateHTTP(); validationError != nil {
		return validationError
	}
	if validationError := configuration.validateIngestion(); validationError != nil {
		return validationError
	}
	if validationError := configuration.validateDatabase(); validationError != nil {
		return validationError
	}
	if validationError := configuration.validateRuntime(); validationError != nil {
		return validationError
	}
	return nil
}

func (configuration Config) validateHTTP() error {
	if strings.TrimSpace(configuration.HTTP.Address) == "" {
		return errors.New("http.address must not be empty")
	}
	if configuration.HTTP.ReadHeaderTimeout <= 0 || configuration.HTTP.ReadTimeout <= 0 || configuration.HTTP.WriteTimeout <= 0 || configuration.HTTP.IdleTimeout <= 0 {
		return errors.New("HTTP timeouts must be positive")
	}
	if configuration.HTTP.MaxHeaderBytes <= 0 || configuration.HTTP.ManagementMaxBodyBytes <= 0 {
		return errors.New("HTTP byte limits must be positive")
	}
	return nil
}

func (configuration Config) validateIngestion() error {
	if strings.TrimSpace(configuration.Ingestion.GRPCAddress) == "" || strings.TrimSpace(configuration.Ingestion.HTTPAddress) == "" {
		return errors.New("ingestion addresses must not be empty")
	}
	if configuration.HTTP.Address == configuration.Ingestion.GRPCAddress || configuration.HTTP.Address == configuration.Ingestion.HTTPAddress || configuration.Ingestion.GRPCAddress == configuration.Ingestion.HTTPAddress {
		return errors.New("management, OTLP/gRPC, and OTLP/HTTP addresses must be distinct")
	}
	if !validUUID(configuration.Ingestion.ProjectID) || !validUUID(configuration.Ingestion.EnvironmentID) {
		return errors.New("ingestion project_id and environment_id must be UUIDs")
	}
	if !projectSlugPattern.MatchString(configuration.Ingestion.ProjectSlug) {
		return errors.New("ingestion project_slug has an invalid format")
	}
	if strings.TrimSpace(configuration.Ingestion.ProjectName) == "" || strings.TrimSpace(configuration.Ingestion.EnvironmentName) == "" {
		return errors.New("ingestion project_name and environment_name must not be empty")
	}
	if configuration.Ingestion.MaxRequestBytes <= 0 || configuration.Ingestion.MaxRequestBytes > 64<<20 || configuration.Ingestion.MaxRecordsPerRequest <= 0 || configuration.Ingestion.MaxRecordsPerRequest > maxIngestionRecords || configuration.Ingestion.MaxAttributesPerRecord <= 0 || configuration.Ingestion.MaxAttributesPerRecord > maxIngestionAttributes || configuration.Ingestion.MaxAttributeValueBytes <= 0 || configuration.Ingestion.MaxAttributeValueBytes > maxIngestionAttributeBytes || configuration.Ingestion.MaxAttributeDepth <= 0 || configuration.Ingestion.MaxAttributeDepth > 16 {
		return errors.New("ingestion request and attribute limits are invalid")
	}
	if configuration.Ingestion.QueueSize < configuration.Ingestion.BatchSize || configuration.Ingestion.QueueSize > maxIngestionQueueSize || configuration.Ingestion.QueueMaxBytes < configuration.Ingestion.MaxRequestBytes || configuration.Ingestion.QueueMaxBytes > maxIngestionQueueBytes || configuration.Ingestion.BatchSize <= 0 || configuration.Ingestion.BatchSize > maxIngestionBatchSize || configuration.Ingestion.Workers <= 0 || configuration.Ingestion.Workers > maxIngestionWorkers || configuration.Ingestion.EnqueueTimeout <= 0 || configuration.Ingestion.BatchTimeout <= 0 || configuration.Ingestion.FlushInterval <= 0 {
		return errors.New("ingestion queue, batch, worker, and timeout bounds are invalid")
	}
	return nil
}

func (configuration Config) validateDatabase() error {
	if strings.TrimSpace(configuration.Database.URL) == "" {
		return errors.New("database.url or FLIGHT_DATABASE_URL is required")
	}
	if configuration.Database.MaxConnections <= 0 || configuration.Database.MaxConnections > maxDatabaseConnections || configuration.Database.MinConnections < 0 || configuration.Database.MinConnections > configuration.Database.MaxConnections {
		return errors.New("database connections must satisfy 0 <= min_connections <= max_connections")
	}
	if int32(configuration.Ingestion.Workers+2) > configuration.Database.MaxConnections {
		return errors.New("database max_connections must reserve one connection per ingestion worker plus two")
	}
	if configuration.Database.MaxConnectionLifetime <= 0 || configuration.Database.MaxConnectionIdleTime <= 0 || configuration.Database.HealthCheckPeriod <= 0 || configuration.Database.ReadinessTimeout <= 0 {
		return errors.New("database durations must be positive")
	}
	if strings.TrimSpace(configuration.Database.MigrationsPath) == "" {
		return errors.New("database.migrations_path must not be empty")
	}
	return nil
}

func (configuration Config) validateRuntime() error {
	if configuration.Auth.BootstrapAPIKey != "" && len(configuration.Auth.BootstrapAPIKey) < 32 {
		return errors.New("auth.bootstrap_api_key must contain at least 32 characters")
	}
	if (configuration.TLS.CertificateFile == "") != (configuration.TLS.PrivateKeyFile == "") {
		return errors.New("tls.certificate_file and tls.private_key_file must be set together")
	}
	if configuration.Replay.Enabled && len(configuration.Replay.AllowedHosts) == 0 {
		return errors.New("replay.allowed_hosts is required when replay is enabled")
	}
	if configuration.Replay.Timeout <= 0 || configuration.Replay.MaxRequestBytes <= 0 || configuration.Replay.MaxRequestBytes > 4<<20 || configuration.Replay.MaxResponseBytes <= 0 || configuration.Replay.MaxResponseBytes > 4<<20 || configuration.Replay.MaxConcurrent <= 0 || configuration.Replay.MaxConcurrent > 16 {
		return errors.New("replay limits are invalid")
	}
	if configuration.Retention.MaxAge < 0 || configuration.Retention.CheckInterval <= 0 || configuration.Retention.BatchSize <= 0 || configuration.Retention.BatchSize > maxIngestionBatchSize {
		return errors.New("retention limits are invalid")
	}
	if configuration.ShutdownTimeout <= 0 {
		return errors.New("shutdown_timeout must be positive")
	}
	return nil
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	nonZero := false
	for index, character := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if !strings.ContainsRune("0123456789abcdefABCDEF", character) {
			return false
		}
		if character != '0' {
			nonZero = true
		}
	}
	return nonZero
}
