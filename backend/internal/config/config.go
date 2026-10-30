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
	maxIngestionAttributeBytes     = 1 << 20
	maxIngestionQueueSize          = 100_000
	maxIngestionConcurrentRequests = 1024
	maxIngestionQueueBytes         = 4 << 30
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
	// AuthFailureLimit is how many failed authentications one client address may
	// make per AuthFailureWindow before OTLP answers without a database lookup.
	// 0 turns the limit off.
	AuthFailureLimit  int           `config:"auth_failure_limit"`
	AuthFailureWindow time.Duration `config:"auth_failure_window"`
	// ScrubFreeText masks credentials inside free text (log bodies, exception
	// messages, URLs) as well as under sensitive attribute names.
	ScrubFreeText bool `config:"scrub_free_text"`
}

type HTTPConfig struct {
	Address        string   `config:"address"`
	AllowedOrigins []string `config:"allowed_origins"`
	TrustedProxies []string `config:"trusted_proxies"`
	// MaxConcurrentQueries bounds the management API requests running at once.
	// They share the database pool with ingestion and the background workers, so
	// the cap keeps a burst of heavy reads from taking every connection.
	MaxConcurrentQueries int `config:"max_concurrent_queries"`
	// RequestTimeout cancels a management API request, and the database work
	// behind it, after this long. Keep it below WriteTimeout so the client gets
	// an answer instead of a closed connection.
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

type DatabaseConfig struct {
	URL                   string        `config:"url"`
	MaxConnections        int32         `config:"max_connections"`
	MinConnections        int32         `config:"min_connections"`
	MaxConnectionLifetime time.Duration `config:"max_connection_lifetime"`
	MaxConnectionIdleTime time.Duration `config:"max_connection_idle_time"`
	HealthCheckPeriod     time.Duration `config:"health_check_period"`
	ReadinessTimeout      time.Duration `config:"readiness_timeout"`
	MigrationsPath        string        `config:"migrations_path" env:"TRACEPILOT_MIGRATIONS_PATH"`
	// Session limits applied to every pooled connection. They keep one runaway
	// query, a lock queue or an abandoned transaction from holding a connection
	// the rest of the service needs. 0 leaves the server default. A URL parameter
	// of the same name wins. Migrations use their own connection and are not capped.
	StatementTimeout         time.Duration `config:"statement_timeout"`
	LockTimeout              time.Duration `config:"lock_timeout"`
	IdleInTransactionTimeout time.Duration `config:"idle_in_transaction_timeout"`
}

type AuthConfig struct {
	BootstrapAPIKey string `config:"bootstrap_api_key"`
}

type TLSConfig struct {
	CertificateFile string `config:"certificate_file"`
	PrivateKeyFile  string `config:"private_key_file"`
}

// RetentionConfig sets how long each group of data is kept; zero keeps the
// group forever. MaxAge covers raw telemetry (events, spans, logs, metric
// points). Incidents expire with their evidence, occurrences, status history
// and replay results, and only once resolved or ignored.
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

// Load builds the configuration from one flow: built-in defaults, then the
// first .env file found (see readEnvironmentFile), then the process
// environment, which wins. Every field maps to TRACEPILOT_<SECTION>_<FIELD>.
func Load() (Config, error) {
	fileValues, fileError := readEnvironmentFile()
	if fileError != nil {
		return Config{}, fileError
	}
	configuration := defaultConfig()
	if environmentError := applyEnvironment(&configuration, fileValues); environmentError != nil {
		return Config{}, environmentError
	}
	if validationError := configuration.Validate(); validationError != nil {
		return Config{}, validationError
	}
	return configuration, nil
}

// Native commands also run from backend/ via go -C; only a missing local file
// permits falling back to the repository-root .env.
func readEnvironmentFile() (map[string]string, error) {
	for _, path := range []string{".env", "../.env"} {
		values, err := godotenv.Read(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			// Parser errors can contain secret values from the input line.
			return nil, fmt.Errorf("read %s: invalid or unreadable environment file", path)
		}
		return values, nil
	}
	return nil, nil
}
