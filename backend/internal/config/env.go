package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

func applyEnvironment(configuration *Config) error {
	var parseError error
	configuration.HTTP.Address = environmentValue("TRACEPILOT_HTTP_ADDRESS", configuration.HTTP.Address)
	if allowedOrigins, exists := os.LookupEnv("TRACEPILOT_HTTP_ALLOWED_ORIGINS"); exists {
		configuration.HTTP.AllowedOrigins = splitCommaSeparated(allowedOrigins)
	}
	configuration.Ingestion.GRPCAddress = environmentValue("TRACEPILOT_INGESTION_GRPC_ADDRESS", configuration.Ingestion.GRPCAddress)
	configuration.Ingestion.HTTPAddress = environmentValue("TRACEPILOT_INGESTION_HTTP_ADDRESS", configuration.Ingestion.HTTPAddress)
	configuration.Ingestion.ProjectID = environmentValue("TRACEPILOT_INGESTION_PROJECT_ID", configuration.Ingestion.ProjectID)
	configuration.Ingestion.ProjectSlug = environmentValue("TRACEPILOT_INGESTION_PROJECT_SLUG", configuration.Ingestion.ProjectSlug)
	configuration.Ingestion.ProjectName = environmentValue("TRACEPILOT_INGESTION_PROJECT_NAME", configuration.Ingestion.ProjectName)
	configuration.Ingestion.EnvironmentID = environmentValue("TRACEPILOT_INGESTION_ENVIRONMENT_ID", configuration.Ingestion.EnvironmentID)
	configuration.Ingestion.EnvironmentName = environmentValue("TRACEPILOT_INGESTION_ENVIRONMENT_NAME", configuration.Ingestion.EnvironmentName)
	configuration.Database.URL = environmentValue("TRACEPILOT_DATABASE_URL", configuration.Database.URL)
	configuration.Database.MigrationsPath = environmentValue("TRACEPILOT_MIGRATIONS_PATH", configuration.Database.MigrationsPath)
	configuration.Auth.BootstrapAPIKey = environmentValue("TRACEPILOT_AUTH_BOOTSTRAP_API_KEY", configuration.Auth.BootstrapAPIKey)
	configuration.TLS.CertificateFile = environmentValue("TRACEPILOT_TLS_CERTIFICATE_FILE", configuration.TLS.CertificateFile)
	configuration.TLS.PrivateKeyFile = environmentValue("TRACEPILOT_TLS_PRIVATE_KEY_FILE", configuration.TLS.PrivateKeyFile)
	if configuration.Production, parseError = readBoolean("TRACEPILOT_PRODUCTION", configuration.Production); parseError != nil {
		return parseError
	}
	if configuration.Replay.Enabled, parseError = readBoolean("TRACEPILOT_REPLAY_ENABLED", configuration.Replay.Enabled); parseError != nil {
		return parseError
	}
	if replayAllowedHosts, exists := os.LookupEnv("TRACEPILOT_REPLAY_ALLOWED_HOSTS"); exists {
		configuration.Replay.AllowedHosts = splitCommaSeparated(replayAllowedHosts)
	}
	if configuration.Replay.AllowPrivate, parseError = readBoolean("TRACEPILOT_REPLAY_ALLOW_PRIVATE", configuration.Replay.AllowPrivate); parseError != nil {
		return parseError
	}
	if configuration.Retention.MaxAge, parseError = readNonNegativeDuration("TRACEPILOT_RETENTION_MAX_AGE", configuration.Retention.MaxAge); parseError != nil {
		return parseError
	}

	if configuration.HTTP.ReadHeaderTimeout, parseError = readDuration("TRACEPILOT_HTTP_READ_HEADER_TIMEOUT", configuration.HTTP.ReadHeaderTimeout); parseError != nil {
		return parseError
	}
	if configuration.HTTP.ReadTimeout, parseError = readDuration("TRACEPILOT_HTTP_READ_TIMEOUT", configuration.HTTP.ReadTimeout); parseError != nil {
		return parseError
	}
	if configuration.HTTP.WriteTimeout, parseError = readDuration("TRACEPILOT_HTTP_WRITE_TIMEOUT", configuration.HTTP.WriteTimeout); parseError != nil {
		return parseError
	}
	if configuration.HTTP.IdleTimeout, parseError = readDuration("TRACEPILOT_HTTP_IDLE_TIMEOUT", configuration.HTTP.IdleTimeout); parseError != nil {
		return parseError
	}
	if configuration.ShutdownTimeout, parseError = readDuration("TRACEPILOT_SHUTDOWN_TIMEOUT", configuration.ShutdownTimeout); parseError != nil {
		return parseError
	}
	if configuration.Replay.Timeout, parseError = readDuration("TRACEPILOT_REPLAY_TIMEOUT", configuration.Replay.Timeout); parseError != nil {
		return parseError
	}
	if configuration.Retention.CheckInterval, parseError = readDuration("TRACEPILOT_RETENTION_CHECK_INTERVAL", configuration.Retention.CheckInterval); parseError != nil {
		return parseError
	}
	if configuration.HTTP.MaxHeaderBytes, parseError = readPositiveInteger("TRACEPILOT_HTTP_MAX_HEADER_BYTES", configuration.HTTP.MaxHeaderBytes); parseError != nil {
		return parseError
	}
	if configuration.HTTP.ManagementMaxBodyBytes, parseError = readPositiveInt64("TRACEPILOT_HTTP_MANAGEMENT_MAX_BODY_BYTES", configuration.HTTP.ManagementMaxBodyBytes); parseError != nil {
		return parseError
	}
	if configuration.Replay.MaxResponseBytes, parseError = readPositiveInt64("TRACEPILOT_REPLAY_MAX_RESPONSE_BYTES", configuration.Replay.MaxResponseBytes); parseError != nil {
		return parseError
	}
	if configuration.Replay.MaxRequestBytes, parseError = readPositiveInt64("TRACEPILOT_REPLAY_MAX_REQUEST_BYTES", configuration.Replay.MaxRequestBytes); parseError != nil {
		return parseError
	}
	if configuration.Replay.MaxConcurrent, parseError = readPositiveInteger("TRACEPILOT_REPLAY_MAX_CONCURRENT", configuration.Replay.MaxConcurrent); parseError != nil {
		return parseError
	}
	if configuration.Retention.BatchSize, parseError = readPositiveInteger("TRACEPILOT_RETENTION_BATCH_SIZE", configuration.Retention.BatchSize); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.MaxRequestBytes, parseError = readPositiveInt64("TRACEPILOT_INGESTION_MAX_REQUEST_BYTES", configuration.Ingestion.MaxRequestBytes); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.MaxRecordsPerRequest, parseError = readPositiveInteger("TRACEPILOT_INGESTION_MAX_RECORDS_PER_REQUEST", configuration.Ingestion.MaxRecordsPerRequest); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.MaxAttributesPerRecord, parseError = readPositiveInteger("TRACEPILOT_INGESTION_MAX_ATTRIBUTES_PER_RECORD", configuration.Ingestion.MaxAttributesPerRecord); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.MaxAttributeValueBytes, parseError = readPositiveInteger("TRACEPILOT_INGESTION_MAX_ATTRIBUTE_VALUE_BYTES", configuration.Ingestion.MaxAttributeValueBytes); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.MaxAttributeDepth, parseError = readPositiveInteger("TRACEPILOT_INGESTION_MAX_ATTRIBUTE_DEPTH", configuration.Ingestion.MaxAttributeDepth); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.QueueSize, parseError = readPositiveInteger("TRACEPILOT_INGESTION_QUEUE_SIZE", configuration.Ingestion.QueueSize); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.QueueMaxBytes, parseError = readPositiveInt64("TRACEPILOT_INGESTION_QUEUE_MAX_BYTES", configuration.Ingestion.QueueMaxBytes); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.EnqueueTimeout, parseError = readDuration("TRACEPILOT_INGESTION_ENQUEUE_TIMEOUT", configuration.Ingestion.EnqueueTimeout); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.BatchSize, parseError = readPositiveInteger("TRACEPILOT_INGESTION_BATCH_SIZE", configuration.Ingestion.BatchSize); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.BatchTimeout, parseError = readDuration("TRACEPILOT_INGESTION_BATCH_TIMEOUT", configuration.Ingestion.BatchTimeout); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.FlushInterval, parseError = readDuration("TRACEPILOT_INGESTION_FLUSH_INTERVAL", configuration.Ingestion.FlushInterval); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.Workers, parseError = readPositiveInteger("TRACEPILOT_INGESTION_WORKERS", configuration.Ingestion.Workers); parseError != nil {
		return parseError
	}
	if redactedKeys, exists := os.LookupEnv("TRACEPILOT_INGESTION_REDACTED_KEYS"); exists {
		configuration.Ingestion.RedactedKeys = splitCommaSeparated(redactedKeys)
	}
	if configuration.Database.MaxConnections, parseError = readPositiveInt32("TRACEPILOT_DATABASE_MAX_CONNECTIONS", configuration.Database.MaxConnections); parseError != nil {
		return parseError
	}
	if configuration.Database.MinConnections, parseError = readNonNegativeInt32("TRACEPILOT_DATABASE_MIN_CONNECTIONS", configuration.Database.MinConnections); parseError != nil {
		return parseError
	}
	if configuration.Database.MaxConnectionLifetime, parseError = readDuration("TRACEPILOT_DATABASE_MAX_CONNECTION_LIFETIME", configuration.Database.MaxConnectionLifetime); parseError != nil {
		return parseError
	}
	if configuration.Database.MaxConnectionIdleTime, parseError = readDuration("TRACEPILOT_DATABASE_MAX_CONNECTION_IDLE_TIME", configuration.Database.MaxConnectionIdleTime); parseError != nil {
		return parseError
	}
	if configuration.Database.HealthCheckPeriod, parseError = readDuration("TRACEPILOT_DATABASE_HEALTH_CHECK_PERIOD", configuration.Database.HealthCheckPeriod); parseError != nil {
		return parseError
	}
	if configuration.Database.ReadinessTimeout, parseError = readDuration("TRACEPILOT_DATABASE_READINESS_TIMEOUT", configuration.Database.ReadinessTimeout); parseError != nil {
		return parseError
	}
	if configuration.LogLevel, parseError = parseLogLevel(environmentValue("TRACEPILOT_LOG_LEVEL", configuration.LogLevel.String())); parseError != nil {
		return parseError
	}
	return nil
}

func splitCommaSeparated(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func environmentValue(name, fallback string) string {
	if value, exists := os.LookupEnv(name); exists {
		return value
	}
	return fallback
}

func readDuration(name string, fallback time.Duration) (time.Duration, error) {
	rawValue, exists := os.LookupEnv(name)
	if !exists {
		return fallback, nil
	}
	parsedDuration, parseError := time.ParseDuration(rawValue)
	if parseError != nil || parsedDuration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return parsedDuration, nil
}

func readNonNegativeDuration(name string, fallback time.Duration) (time.Duration, error) {
	rawValue, exists := os.LookupEnv(name)
	if !exists {
		return fallback, nil
	}
	parsedDuration, parseError := time.ParseDuration(rawValue)
	if parseError != nil || parsedDuration < 0 {
		return 0, fmt.Errorf("%s must be a non-negative duration", name)
	}
	return parsedDuration, nil
}

func readBoolean(name string, fallback bool) (bool, error) {
	rawValue, exists := os.LookupEnv(name)
	if !exists {
		return fallback, nil
	}
	parsed, parseError := strconv.ParseBool(rawValue)
	if parseError != nil {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return parsed, nil
}

func readPositiveInteger(name string, fallback int) (int, error) {
	rawValue, exists := os.LookupEnv(name)
	if !exists {
		return fallback, nil
	}
	parsedInteger, parseError := strconv.Atoi(rawValue)
	if parseError != nil || parsedInteger <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsedInteger, nil
}

func readPositiveInt64(name string, fallback int64) (int64, error) {
	rawValue, exists := os.LookupEnv(name)
	if !exists {
		return fallback, nil
	}
	parsedInteger, parseError := strconv.ParseInt(rawValue, 10, 64)
	if parseError != nil || parsedInteger <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsedInteger, nil
}

func readPositiveInt32(name string, fallback int32) (int32, error) {
	parsedInteger, parseError := readInt32(name, fallback)
	if parseError != nil || parsedInteger <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsedInteger, nil
}

func readNonNegativeInt32(name string, fallback int32) (int32, error) {
	parsedInteger, parseError := readInt32(name, fallback)
	if parseError != nil || parsedInteger < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return parsedInteger, nil
}

func readInt32(name string, fallback int32) (int32, error) {
	rawValue, exists := os.LookupEnv(name)
	if !exists {
		return fallback, nil
	}
	parsedInteger, parseError := strconv.ParseInt(rawValue, 10, 32)
	return int32(parsedInteger), parseError
}

func parseLogLevel(value string) (slog.Level, error) {
	var level slog.Level
	if parseError := level.UnmarshalText([]byte(strings.ToUpper(value))); parseError != nil {
		return 0, errors.New("TRACEPILOT_LOG_LEVEL/log_level must be debug, info, warn, or error")
	}
	return level, nil
}
