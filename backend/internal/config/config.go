package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/joho/godotenv"
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
	maxDatabaseConnections         = 1_000
)

type Config struct {
	Production      bool                `config:"production"`
	HTTP            HTTPConfig          `config:"http"`
	Ingestion       IngestionConfig     `config:"ingestion"`
	Database        DatabaseConfig      `config:"database"`
	Auth            AuthConfig          `config:"auth"`
	TLS             TLSConfig           `config:"tls"`
	Retention       RetentionConfig     `config:"retention"`
	Replay          ReplayConfig        `config:"replay"`
	Notifications   NotificationsConfig `config:"notifications"`
	ShutdownTimeout time.Duration       `config:"shutdown_timeout"`
	LogLevel        slog.Level          `config:"log_level"`
}

type HTTPConfig struct {
	// MetricsToken lets a metrics scraper read /metrics in production without a person's sign-in.
	MetricsToken   string   `config:"metrics_token"`
	Address        string   `config:"address"`
	AllowedOrigins []string `config:"allowed_origins"`
	TrustedProxies []string `config:"trusted_proxies"`

	// Limits concurrent management requests to protect the shared DB pool.
	MaxConcurrentQueries int `config:"max_concurrent_queries"`

	// RequestTimeout should be shorter than WriteTimeout.
	RequestTimeout          time.Duration `config:"request_timeout"`
	ReadHeaderTimeout       time.Duration `config:"read_header_timeout"`
	ReadTimeout             time.Duration `config:"read_timeout"`
	WriteTimeout            time.Duration `config:"write_timeout"`
	IdleTimeout             time.Duration `config:"idle_timeout"`
	MaxHeaderBytes          int           `config:"max_header_bytes"`
	ManagementMaxBodyBytes  int64         `config:"management_max_body_bytes"`
	RateLimitRequests       int           `config:"rate_limit_requests"`
	RateLimitWindow         time.Duration `config:"rate_limit_window"`
	RateLimitAuthRequests   int           `config:"rate_limit_auth_requests"`
	RateLimitReplayRequests int           `config:"rate_limit_replay_requests"`
}

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

type DatabaseConfig struct {
	URL                   string        `config:"url"`
	MaxConnections        int32         `config:"max_connections"`
	MinConnections        int32         `config:"min_connections"`
	MaxConnectionLifetime time.Duration `config:"max_connection_lifetime"`
	MaxConnectionIdleTime time.Duration `config:"max_connection_idle_time"`
	HealthCheckPeriod     time.Duration `config:"health_check_period"`
	ReadinessTimeout      time.Duration `config:"readiness_timeout"`
	MigrationsPath        string        `config:"migrations_path" env:"TRACEPILOT_MIGRATIONS_PATH"`

	// PostgreSQL session limits for pooled connections.
	// Zero leaves the PostgreSQL server default.
	StatementTimeout         time.Duration `config:"statement_timeout"`
	LockTimeout              time.Duration `config:"lock_timeout"`
	IdleInTransactionTimeout time.Duration `config:"idle_in_transaction_timeout"`
}

type AuthConfig struct {
	// JWTSecret signs console access tokens and ingest tokens. Without one, a random secret is made at
	// startup, so tokens stop working on restart and replicas do not accept each other's.
	JWTSecret string `config:"jwt_secret"`
	// IngestJWTSecret signs and validates telemetry ingest tokens independently from console user tokens.
	// When unset, it falls back to JWTSecret.
	IngestJWTSecret string `config:"ingest_jwt_secret"`
	// AllowRegistration lets anyone create an account. The first account is always allowed and becomes
	// the super administrator.
	AllowRegistration bool `config:"allow_registration"`
}

// EffectiveIngestJWTSecret returns IngestJWTSecret if configured, else falls back to JWTSecret.
func (auth AuthConfig) EffectiveIngestJWTSecret() string {
	if auth.IngestJWTSecret != "" {
		return auth.IngestJWTSecret
	}
	return auth.JWTSecret
}

type TLSConfig struct {
	CertificateFile string `config:"certificate_file"`
	PrivateKeyFile  string `config:"private_key_file"`
}

// RetentionConfig controls data expiration.
// A zero MaxAge keeps the corresponding data indefinitely.
type RetentionConfig struct {
	MaxAge         time.Duration `config:"max_age"`
	IncidentMaxAge time.Duration `config:"incident_max_age"`
	AuditMaxAge    time.Duration `config:"audit_max_age"`
	DeliveryMaxAge time.Duration `config:"delivery_max_age"`
	CheckInterval  time.Duration `config:"check_interval"`
	BatchSize      int           `config:"batch_size"`
}

type ReplayConfig struct {
	Enabled          bool          `config:"enabled"`
	AllowedHosts     []string      `config:"allowed_hosts"`
	AllowPrivate     bool          `config:"allow_private"`
	Timeout          time.Duration `config:"timeout"`
	MaxRequestBytes  int64         `config:"max_request_bytes"`
	MaxResponseBytes int64         `config:"max_response_bytes"`
	MaxConcurrent    int           `config:"max_concurrent"`
}

type NotificationsConfig struct {
	Enabled        bool          `config:"enabled"`
	EncryptionKey  string        `config:"encryption_key" env:"TRACEPILOT_WEBHOOK_ENCRYPTION_KEY"`
	PollInterval   time.Duration `config:"poll_interval" env:"TRACEPILOT_WEBHOOK_POLL_INTERVAL"`
	Workers        int           `config:"workers" env:"TRACEPILOT_WEBHOOK_WORKERS"`
	RequestTimeout time.Duration `config:"request_timeout" env:"TRACEPILOT_WEBHOOK_REQUEST_TIMEOUT"`
}

// Load initializes configuration using the following precedence:
//
// 1. Built-in defaults.
// 2. Environment file.
// 3. Process environment variables.
//
// Configuration is validated before being returned.
func Load() (Config, error) {
	fileValues, err := readEnvironmentFile()
	if err != nil {
		return Config{}, err
	}

	cfg := defaultConfig()

	if err := applyEnvironment(&cfg, fileValues); err != nil {
		return Config{}, fmt.Errorf("apply configuration: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate configuration: %w", err)
	}

	return cfg, nil
}

// readEnvironmentFile reads the first available environment file.
//
// The lookup order supports both:
//
//   - Running from the repository root.
//   - Running commands from backend/.
//
// A parent directory is checked only when the local file does not exist.
// Invalid or unreadable files are returned as errors.
func readEnvironmentFile() (map[string]string, error) {
	paths := [...]string{
		".env",
		"../.env",
	}

	for _, path := range paths {
		values, err := godotenv.Read(path)
		if err == nil {
			return values, nil
		}

		if errors.Is(err, os.ErrNotExist) {
			continue
		}

		// Do not expose potentially sensitive values from parser errors.
		return nil, fmt.Errorf(
			"read environment file %q: invalid or unreadable file",
			path,
		)
	}

	return nil, nil
}
