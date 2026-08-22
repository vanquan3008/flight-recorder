package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/joho/godotenv"
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
	Auth            AuthConfig      `yaml:"auth"`
	TLS             TLSConfig       `yaml:"tls"`
	Retention       RetentionConfig `yaml:"retention"`
	Replay          ReplayConfig    `yaml:"replay"`
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

type AuthConfig struct {
	BootstrapAPIKey string `yaml:"bootstrap_api_key"`
}

type TLSConfig struct {
	CertificateFile string `yaml:"certificate_file"`
	PrivateKeyFile  string `yaml:"private_key_file"`
}

type RetentionConfig struct {
	MaxAge        time.Duration `yaml:"max_age"`
	CheckInterval time.Duration `yaml:"check_interval"`
	BatchSize     int           `yaml:"batch_size"`
}

type ReplayConfig struct {
	Enabled          bool          `yaml:"enabled"`
	AllowedHosts     []string      `yaml:"allowed_hosts"`
	AllowPrivate     bool          `yaml:"allow_private"`
	Timeout          time.Duration `yaml:"timeout"`
	MaxRequestBytes  int64         `yaml:"max_request_bytes"`
	MaxResponseBytes int64         `yaml:"max_response_bytes"`
	MaxConcurrent    int           `yaml:"max_concurrent"`
}

func Load(configFile string) (Config, error) {
	_ = godotenv.Load()

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
