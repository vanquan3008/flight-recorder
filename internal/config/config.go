package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const (
	CurrentVersion             = 1
	maxConfigFileSize          = 1 << 20
	maxIngestionRecords        = 100_000
	maxIngestionAttributes     = 1_024
	maxIngestionAttributeBytes = 1 << 20
	maxIngestionQueueSize      = 100_000
	maxIngestionQueueBytes     = 4 << 30
	maxIngestionBatchSize      = 10_000
	maxIngestionWorkers        = 64
	maxDatabaseConnections     = 1_000
)

type Config struct {
	Version         int             `yaml:"version"`
	HTTP            HTTPConfig      `yaml:"http"`
	Ingestion       IngestionConfig `yaml:"ingestion"`
	Database        DatabaseConfig  `yaml:"database"`
	ShutdownTimeout time.Duration   `yaml:"shutdown_timeout"`
	LogLevel        slog.Level      `yaml:"log_level"`
}

type IngestionConfig struct {
	GRPCAddress            string        `yaml:"grpc_address"`
	HTTPAddress            string        `yaml:"http_address"`
	ProjectID              string        `yaml:"project_id"`
	ProjectSlug            string        `yaml:"project_slug"`
	ProjectName            string        `yaml:"project_name"`
	EnvironmentID          string        `yaml:"environment_id"`
	EnvironmentName        string        `yaml:"environment_name"`
	MaxRequestBytes        int64         `yaml:"max_request_bytes"`
	MaxRecordsPerRequest   int           `yaml:"max_records_per_request"`
	MaxAttributesPerRecord int           `yaml:"max_attributes_per_record"`
	MaxAttributeValueBytes int           `yaml:"max_attribute_value_bytes"`
	MaxAttributeDepth      int           `yaml:"max_attribute_depth"`
	QueueSize              int           `yaml:"queue_size"`
	QueueMaxBytes          int64         `yaml:"queue_max_bytes"`
	EnqueueTimeout         time.Duration `yaml:"enqueue_timeout"`
	BatchSize              int           `yaml:"batch_size"`
	BatchTimeout           time.Duration `yaml:"batch_timeout"`
	FlushInterval          time.Duration `yaml:"flush_interval"`
	Workers                int           `yaml:"workers"`
	RedactedKeys           []string      `yaml:"redacted_keys"`
}

type HTTPConfig struct {
	Address                string        `yaml:"address"`
	ReadHeaderTimeout      time.Duration `yaml:"read_header_timeout"`
	ReadTimeout            time.Duration `yaml:"read_timeout"`
	WriteTimeout           time.Duration `yaml:"write_timeout"`
	IdleTimeout            time.Duration `yaml:"idle_timeout"`
	MaxHeaderBytes         int           `yaml:"max_header_bytes"`
	ManagementMaxBodyBytes int64         `yaml:"management_max_body_bytes"`
}

type DatabaseConfig struct {
	URL                   string        `yaml:"url"`
	MaxConnections        int32         `yaml:"max_connections"`
	MinConnections        int32         `yaml:"min_connections"`
	MaxConnectionLifetime time.Duration `yaml:"max_connection_lifetime"`
	MaxConnectionIdleTime time.Duration `yaml:"max_connection_idle_time"`
	HealthCheckPeriod     time.Duration `yaml:"health_check_period"`
	ReadinessTimeout      time.Duration `yaml:"readiness_timeout"`
	MigrationsPath        string        `yaml:"migrations_path"`
}

func Load(configFile string) (Config, error) {
	configuration := defaultConfig()
	if configFile == "" {
		configFile = os.Getenv("FLIGHT_CONFIG_FILE")
	}
	if configFile != "" {
		configuration.Version = 0
		if decodeError := decodeFile(configFile, &configuration); decodeError != nil {
			return Config{}, decodeError
		}
	}
	if environmentError := applyEnvironment(&configuration); environmentError != nil {
		return Config{}, environmentError
	}
	if validationError := configuration.Validate(); validationError != nil {
		return Config{}, validationError
	}
	return configuration, nil
}

func defaultConfig() Config {
	return Config{
		Version: CurrentVersion,
		HTTP: HTTPConfig{
			Address:                ":8080",
			ReadHeaderTimeout:      5 * time.Second,
			ReadTimeout:            15 * time.Second,
			WriteTimeout:           30 * time.Second,
			IdleTimeout:            60 * time.Second,
			MaxHeaderBytes:         1 << 20,
			ManagementMaxBodyBytes: 1 << 20,
		},
		Ingestion: IngestionConfig{
			GRPCAddress:            ":4317",
			HTTPAddress:            ":4318",
			ProjectID:              "01900000-0000-7000-8000-000000000001",
			ProjectSlug:            "default",
			ProjectName:            "Default Project",
			EnvironmentID:          "01900000-0000-7000-8000-000000000002",
			EnvironmentName:        "development",
			MaxRequestBytes:        4 << 20,
			MaxRecordsPerRequest:   10_000,
			MaxAttributesPerRecord: 128,
			MaxAttributeValueBytes: 4 << 10,
			MaxAttributeDepth:      4,
			QueueSize:              4_096,
			QueueMaxBytes:          64 << 20,
			EnqueueTimeout:         250 * time.Millisecond,
			BatchSize:              256,
			BatchTimeout:           5 * time.Second,
			FlushInterval:          200 * time.Millisecond,
			Workers:                2,
		},
		Database: DatabaseConfig{
			MaxConnections:        20,
			MinConnections:        2,
			MaxConnectionLifetime: time.Hour,
			MaxConnectionIdleTime: 30 * time.Minute,
			HealthCheckPeriod:     time.Minute,
			ReadinessTimeout:      2 * time.Second,
			MigrationsPath:        "./migrations",
		},
		ShutdownTimeout: 15 * time.Second,
		LogLevel:        slog.LevelInfo,
	}
}

func decodeFile(configFile string, configuration *Config) error {
	file, openError := os.Open(configFile)
	if openError != nil {
		return fmt.Errorf("open configuration file: %w", openError)
	}
	defer file.Close()

	contents, readError := io.ReadAll(io.LimitReader(file, maxConfigFileSize+1))
	if readError != nil {
		return fmt.Errorf("read configuration file: %w", readError)
	}
	if len(contents) > maxConfigFileSize {
		return fmt.Errorf("configuration file exceeds %d bytes", maxConfigFileSize)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	if decodeError := decoder.Decode(configuration); decodeError != nil {
		return fmt.Errorf("decode configuration file: %w", decodeError)
	}
	if trailingError := decoder.Decode(&struct{}{}); !errors.Is(trailingError, io.EOF) {
		if trailingError == nil {
			return errors.New("configuration file must contain exactly one YAML document")
		}
		return fmt.Errorf("decode trailing configuration: %w", trailingError)
	}
	return nil
}

func applyEnvironment(configuration *Config) error {
	configuration.HTTP.Address = environmentValue("FLIGHT_HTTP_ADDRESS", configuration.HTTP.Address)
	configuration.Ingestion.GRPCAddress = environmentValue("FLIGHT_INGESTION_GRPC_ADDRESS", configuration.Ingestion.GRPCAddress)
	configuration.Ingestion.HTTPAddress = environmentValue("FLIGHT_INGESTION_HTTP_ADDRESS", configuration.Ingestion.HTTPAddress)
	configuration.Ingestion.ProjectID = environmentValue("FLIGHT_INGESTION_PROJECT_ID", configuration.Ingestion.ProjectID)
	configuration.Ingestion.ProjectSlug = environmentValue("FLIGHT_INGESTION_PROJECT_SLUG", configuration.Ingestion.ProjectSlug)
	configuration.Ingestion.ProjectName = environmentValue("FLIGHT_INGESTION_PROJECT_NAME", configuration.Ingestion.ProjectName)
	configuration.Ingestion.EnvironmentID = environmentValue("FLIGHT_INGESTION_ENVIRONMENT_ID", configuration.Ingestion.EnvironmentID)
	configuration.Ingestion.EnvironmentName = environmentValue("FLIGHT_INGESTION_ENVIRONMENT_NAME", configuration.Ingestion.EnvironmentName)
	configuration.Database.URL = environmentValue("FLIGHT_DATABASE_URL", configuration.Database.URL)
	configuration.Database.MigrationsPath = environmentValue("FLIGHT_MIGRATIONS_PATH", configuration.Database.MigrationsPath)

	var parseError error
	if configuration.HTTP.ReadHeaderTimeout, parseError = readDuration("FLIGHT_HTTP_READ_HEADER_TIMEOUT", configuration.HTTP.ReadHeaderTimeout); parseError != nil {
		return parseError
	}
	if configuration.HTTP.ReadTimeout, parseError = readDuration("FLIGHT_HTTP_READ_TIMEOUT", configuration.HTTP.ReadTimeout); parseError != nil {
		return parseError
	}
	if configuration.HTTP.WriteTimeout, parseError = readDuration("FLIGHT_HTTP_WRITE_TIMEOUT", configuration.HTTP.WriteTimeout); parseError != nil {
		return parseError
	}
	if configuration.HTTP.IdleTimeout, parseError = readDuration("FLIGHT_HTTP_IDLE_TIMEOUT", configuration.HTTP.IdleTimeout); parseError != nil {
		return parseError
	}
	if configuration.ShutdownTimeout, parseError = readDuration("FLIGHT_SHUTDOWN_TIMEOUT", configuration.ShutdownTimeout); parseError != nil {
		return parseError
	}
	if configuration.HTTP.MaxHeaderBytes, parseError = readPositiveInteger("FLIGHT_HTTP_MAX_HEADER_BYTES", configuration.HTTP.MaxHeaderBytes); parseError != nil {
		return parseError
	}
	if configuration.HTTP.ManagementMaxBodyBytes, parseError = readPositiveInt64("FLIGHT_HTTP_MANAGEMENT_MAX_BODY_BYTES", configuration.HTTP.ManagementMaxBodyBytes); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.MaxRequestBytes, parseError = readPositiveInt64("FLIGHT_INGESTION_MAX_REQUEST_BYTES", configuration.Ingestion.MaxRequestBytes); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.MaxRecordsPerRequest, parseError = readPositiveInteger("FLIGHT_INGESTION_MAX_RECORDS_PER_REQUEST", configuration.Ingestion.MaxRecordsPerRequest); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.MaxAttributesPerRecord, parseError = readPositiveInteger("FLIGHT_INGESTION_MAX_ATTRIBUTES_PER_RECORD", configuration.Ingestion.MaxAttributesPerRecord); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.MaxAttributeValueBytes, parseError = readPositiveInteger("FLIGHT_INGESTION_MAX_ATTRIBUTE_VALUE_BYTES", configuration.Ingestion.MaxAttributeValueBytes); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.MaxAttributeDepth, parseError = readPositiveInteger("FLIGHT_INGESTION_MAX_ATTRIBUTE_DEPTH", configuration.Ingestion.MaxAttributeDepth); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.QueueSize, parseError = readPositiveInteger("FLIGHT_INGESTION_QUEUE_SIZE", configuration.Ingestion.QueueSize); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.QueueMaxBytes, parseError = readPositiveInt64("FLIGHT_INGESTION_QUEUE_MAX_BYTES", configuration.Ingestion.QueueMaxBytes); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.EnqueueTimeout, parseError = readDuration("FLIGHT_INGESTION_ENQUEUE_TIMEOUT", configuration.Ingestion.EnqueueTimeout); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.BatchSize, parseError = readPositiveInteger("FLIGHT_INGESTION_BATCH_SIZE", configuration.Ingestion.BatchSize); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.BatchTimeout, parseError = readDuration("FLIGHT_INGESTION_BATCH_TIMEOUT", configuration.Ingestion.BatchTimeout); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.FlushInterval, parseError = readDuration("FLIGHT_INGESTION_FLUSH_INTERVAL", configuration.Ingestion.FlushInterval); parseError != nil {
		return parseError
	}
	if configuration.Ingestion.Workers, parseError = readPositiveInteger("FLIGHT_INGESTION_WORKERS", configuration.Ingestion.Workers); parseError != nil {
		return parseError
	}
	if redactedKeys, exists := os.LookupEnv("FLIGHT_INGESTION_REDACTED_KEYS"); exists {
		configuration.Ingestion.RedactedKeys = splitCommaSeparated(redactedKeys)
	}
	if configuration.Database.MaxConnections, parseError = readPositiveInt32("FLIGHT_DATABASE_MAX_CONNECTIONS", configuration.Database.MaxConnections); parseError != nil {
		return parseError
	}
	if configuration.Database.MinConnections, parseError = readNonNegativeInt32("FLIGHT_DATABASE_MIN_CONNECTIONS", configuration.Database.MinConnections); parseError != nil {
		return parseError
	}
	if configuration.Database.MaxConnectionLifetime, parseError = readDuration("FLIGHT_DATABASE_MAX_CONNECTION_LIFETIME", configuration.Database.MaxConnectionLifetime); parseError != nil {
		return parseError
	}
	if configuration.Database.MaxConnectionIdleTime, parseError = readDuration("FLIGHT_DATABASE_MAX_CONNECTION_IDLE_TIME", configuration.Database.MaxConnectionIdleTime); parseError != nil {
		return parseError
	}
	if configuration.Database.HealthCheckPeriod, parseError = readDuration("FLIGHT_DATABASE_HEALTH_CHECK_PERIOD", configuration.Database.HealthCheckPeriod); parseError != nil {
		return parseError
	}
	if configuration.Database.ReadinessTimeout, parseError = readDuration("FLIGHT_DATABASE_READINESS_TIMEOUT", configuration.Database.ReadinessTimeout); parseError != nil {
		return parseError
	}
	if configuration.LogLevel, parseError = parseLogLevel(environmentValue("FLIGHT_LOG_LEVEL", configuration.LogLevel.String())); parseError != nil {
		return parseError
	}
	return nil
}

func (configuration Config) Validate() error {
	if configuration.Version != CurrentVersion {
		return fmt.Errorf("configuration version must be %d", CurrentVersion)
	}
	if strings.TrimSpace(configuration.HTTP.Address) == "" {
		return errors.New("http.address must not be empty")
	}
	if configuration.HTTP.ReadHeaderTimeout <= 0 || configuration.HTTP.ReadTimeout <= 0 || configuration.HTTP.WriteTimeout <= 0 || configuration.HTTP.IdleTimeout <= 0 {
		return errors.New("HTTP timeouts must be positive")
	}
	if configuration.HTTP.MaxHeaderBytes <= 0 || configuration.HTTP.ManagementMaxBodyBytes <= 0 {
		return errors.New("HTTP byte limits must be positive")
	}
	if strings.TrimSpace(configuration.Ingestion.GRPCAddress) == "" || strings.TrimSpace(configuration.Ingestion.HTTPAddress) == "" {
		return errors.New("ingestion addresses must not be empty")
	}
	if configuration.HTTP.Address == configuration.Ingestion.GRPCAddress || configuration.HTTP.Address == configuration.Ingestion.HTTPAddress || configuration.Ingestion.GRPCAddress == configuration.Ingestion.HTTPAddress {
		return errors.New("management, OTLP/gRPC, and OTLP/HTTP addresses must be distinct")
	}
	if !validUUID(configuration.Ingestion.ProjectID) || !validUUID(configuration.Ingestion.EnvironmentID) {
		return errors.New("ingestion project_id and environment_id must be UUIDs")
	}
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`).MatchString(configuration.Ingestion.ProjectSlug) {
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
	if configuration.ShutdownTimeout <= 0 {
		return errors.New("shutdown_timeout must be positive")
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
		return 0, errors.New("FLIGHT_LOG_LEVEL/log_level must be debug, info, warn, or error")
	}
	return level, nil
}
