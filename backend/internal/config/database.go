package config

import (
	"net/url"
	"strings"
	"time"
)

const maxDatabaseConnections = 1_000

// DatabaseConfig configures the PostgreSQL connection pool and migration runner.
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

func (configuration Config) validateDatabase(v *validator) {
	if strings.TrimSpace(configuration.Database.URL) == "" {
		v.add("database.url", "required via database.url or TRACEPILOT_DATABASE_URL")
	}
	if configuration.Database.MaxConnections <= 0 || configuration.Database.MaxConnections > maxDatabaseConnections {
		v.add("database.max_connections", "must be between 1 and 1000")
	}
	if configuration.Database.MinConnections < 0 {
		v.add("database.min_connections", "must not be negative")
	}
	if configuration.Database.MinConnections > configuration.Database.MaxConnections {
		v.add("database.min_connections", "must not exceed max_connections")
	}
	if int32(configuration.Ingestion.Workers+2) > configuration.Database.MaxConnections {
		v.add("database.max_connections", "must reserve at least one connection per ingestion worker plus two")
	}
	if configuration.Database.MaxConnectionLifetime <= 0 {
		v.add("database.max_connection_lifetime", "must be positive")
	}
	if configuration.Database.MaxConnectionIdleTime <= 0 {
		v.add("database.max_connection_idle_time", "must be positive")
	}
	if configuration.Database.HealthCheckPeriod <= 0 {
		v.add("database.health_check_period", "must be positive")
	}
	if configuration.Database.ReadinessTimeout <= 0 {
		v.add("database.readiness_timeout", "must be positive")
	}
	if strings.TrimSpace(configuration.Database.MigrationsPath) == "" {
		v.add("database.migrations_path", "must not be empty")
	}
	if configuration.Production {
		validateProductionDatabaseTLS(configuration.Database.URL, v)
	}
	configuration.validateDatabaseSessionLimits(v)
}

func (configuration Config) validateDatabaseSessionLimits(v *validator) {
	database := configuration.Database
	if database.StatementTimeout < 0 || database.LockTimeout < 0 || database.IdleInTransactionTimeout < 0 {
		v.add("database.session_limits", "statement_timeout, lock_timeout and idle_in_transaction_timeout must not be negative")
	}
	if database.StatementTimeout > 0 && configuration.HTTP.RequestTimeout > 0 && database.StatementTimeout < configuration.HTTP.RequestTimeout {
		v.add("database.statement_timeout", "must not be below http.request_timeout, or queries are cut off before the API can answer")
	}
}

// validateProductionDatabaseTLS rejects database URLs that can fall back to
// plaintext. libpq defaults to sslmode=prefer, so a missing sslmode is rejected
// too. Unix sockets never leave the host and are exempt. require encrypts but
// does not authenticate the server; verify-full is the recommended mode.
func validateProductionDatabaseTLS(databaseURL string, v *validator) {
	parsed, parseError := url.Parse(databaseURL)
	if parseError != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		v.add("database.url", "must be a postgres:// URL in production")
		return
	}
	query := parsed.Query()
	if strings.HasPrefix(parsed.Hostname(), "/") || strings.HasPrefix(query.Get("host"), "/") {
		return
	}
	switch query.Get("sslmode") {
	case "require", "verify-ca", "verify-full":
		return
	}
	v.add("database.url", "must set sslmode to require, verify-ca, or verify-full in production")
}
