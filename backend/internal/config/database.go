package config

import (
	"net/url"
	"strings"
	"time"
)

const maxDatabaseConnections = 1_000

// DatabaseConfig configures PostgreSQL connection pooling, migrations, and session timeouts.
type DatabaseConfig struct {
	URL                   string        `config:"url"`
	MaxConnections        int32         `config:"max_connections"`
	MinConnections        int32         `config:"min_connections"`
	MaxConnectionLifetime time.Duration `config:"max_connection_lifetime"`
	MaxConnectionIdleTime time.Duration `config:"max_connection_idle_time"`
	HealthCheckPeriod     time.Duration `config:"health_check_period"`
	ReadinessTimeout      time.Duration `config:"readiness_timeout"`

	MigrationsPath           string        `config:"migrations_path" env:"TRACEPILOT_MIGRATIONS_PATH"`
	StatementTimeout         time.Duration `config:"statement_timeout"`
	LockTimeout              time.Duration `config:"lock_timeout"`
	IdleInTransactionTimeout time.Duration `config:"idle_in_transaction_timeout"`
}

// validateDatabase validates the PostgreSQL configuration.
func (configuration Config) validateDatabase(validator *validator) {
	database := configuration.Database

	database.validateConnectionPool(validator)
	if int32(configuration.Ingestion.Workers+2) > database.MaxConnections {
		validator.add("database.max_connections", "must reserve at least one connection per ingestion worker plus two")
	}
	database.validateConnectionLifecycle(validator)
	database.validateSessionLimits(validator)
	if database.StatementTimeout > 0 && configuration.HTTP.RequestTimeout > 0 && database.StatementTimeout < configuration.HTTP.RequestTimeout {
		validator.add("database.statement_timeout", "must not be below http.request_timeout, or queries are cut off before the API can answer")
	}
	database.validateMigrations(validator)
	validateDatabaseURL(database.URL, configuration.Production, validator)
}

// validateConnectionPool validates connection pool limits.
func (database DatabaseConfig) validateConnectionPool(validator *validator) {
	if database.MaxConnections < 1 || database.MaxConnections > maxDatabaseConnections {
		validator.add("database.max_connections", "must be between 1 and 1000")
	}
	if database.MinConnections < 0 {
		validator.add("database.min_connections", "must not be negative")
	}
	if database.MinConnections > database.MaxConnections {
		validator.add("database.min_connections", "must not exceed max_connections")
	}
}

// validateConnectionLifecycle validates connection lifetime and health check settings.
func (database DatabaseConfig) validateConnectionLifecycle(validator *validator) {
	if database.MaxConnectionLifetime <= 0 {
		validator.add("database.max_connection_lifetime", "must be positive")
	}
	if database.MaxConnectionIdleTime <= 0 {
		validator.add("database.max_connection_idle_time", "must be positive")
	}
	if database.HealthCheckPeriod <= 0 {
		validator.add("database.health_check_period", "must be positive")
	}
	if database.ReadinessTimeout <= 0 {
		validator.add("database.readiness_timeout", "must be positive")
	}
}

// validateSessionLimits validates PostgreSQL statement, lock, and transaction timeouts.
func (database DatabaseConfig) validateSessionLimits(validator *validator) {
	if database.StatementTimeout < 0 {
		validator.add("database.statement_timeout", "must not be negative")
	}
	if database.LockTimeout < 0 {
		validator.add("database.lock_timeout", "must not be negative")
	}
	if database.IdleInTransactionTimeout < 0 {
		validator.add("database.idle_in_transaction_timeout", "must not be negative")
	}
}

// validateMigrations validates the migration directory configuration.
func (database DatabaseConfig) validateMigrations(validator *validator) {
	if strings.TrimSpace(database.MigrationsPath) == "" {
		validator.add("database.migrations_path", "must not be empty")
	}
}

// validateDatabaseURL validates the PostgreSQL URL and production TLS policy.
func validateDatabaseURL(databaseURL string, production bool, validator *validator) {
	if strings.TrimSpace(databaseURL) == "" {
		validator.add("database.url", "required via database.url or TRACEPILOT_DATABASE_URL")
		return
	}
	parsedURL, parseError := url.Parse(databaseURL)
	if parseError != nil || (parsedURL.Scheme != "postgres" && parsedURL.Scheme != "postgresql") ||
		parsedURL.Opaque != "" || parsedURL.Fragment != "" {
		validator.add("database.url", "must be a valid postgres:// or postgresql:// URL")
		return
	}

	query := parsedURL.Query()
	if query.Has("sslmode") && len(query["sslmode"]) != 1 {
		validator.add("database.url", "sslmode must be specified only once")
		return
	}

	// Unix domain socket connections do not use TLS.
	socketHost := query.Get("host")
	isUnixSocket := strings.HasPrefix(socketHost, "/")

	if !isUnixSocket && parsedURL.Hostname() == "" {
		validator.add("database.url", "database hostname is required")
		return
	}

	if !production || isUnixSocket {
		return
	}
	switch query.Get("sslmode") {
	case "require", "verify-ca", "verify-full":
		return
	default:
		validator.add("database.url", "production requires sslmode=require, verify-ca, or verify-full")
	}
}
