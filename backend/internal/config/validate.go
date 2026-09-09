package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func (configuration Config) Validate() error {
	if configuration.Version != CurrentVersion {
		return fmt.Errorf("configuration version must be %d", CurrentVersion)
	}
	if err := configuration.validateHTTP(); err != nil {
		return err
	}
	if err := configuration.validateIngestion(); err != nil {
		return err
	}
	if err := configuration.validateSecurity(); err != nil {
		return err
	}
	if err := configuration.validateNotifications(); err != nil {
		return err
	}
	if err := configuration.validateDatabase(); err != nil {
		return err
	}
	if err := configuration.validateRuntime(); err != nil {
		return err
	}
	return nil
}

func (configuration Config) validateHTTP() error {
	if strings.TrimSpace(configuration.HTTP.Address) == "" {
		return errors.New("http.address must not be empty")
	}
	if configuration.HTTP.ReadHeaderTimeout <= 0 {
		return errors.New("http.read_header_timeout must be positive")
	}
	if configuration.HTTP.ReadTimeout <= 0 {
		return errors.New("http.read_timeout must be positive")
	}
	if configuration.HTTP.WriteTimeout <= 0 {
		return errors.New("http.write_timeout must be positive")
	}
	if configuration.HTTP.IdleTimeout <= 0 {
		return errors.New("http.idle_timeout must be positive")
	}
	if configuration.HTTP.MaxHeaderBytes <= 0 {
		return errors.New("http.max_header_bytes must be positive")
	}
	if configuration.HTTP.ManagementMaxBodyBytes <= 0 {
		return errors.New("http.management_max_body_bytes must be positive")
	}
	if configuration.HTTP.RateLimitRequests <= 0 {
		return errors.New("http.rate_limit_requests must be positive")
	}
	if configuration.HTTP.RateLimitAuthRequests <= 0 {
		return errors.New("http.rate_limit_auth_requests must be positive")
	}
	if configuration.HTTP.RateLimitReplayRequests <= 0 {
		return errors.New("http.rate_limit_replay_requests must be positive")
	}
	if configuration.HTTP.RateLimitWindow <= 0 {
		return errors.New("http.rate_limit_window must be positive")
	}
	for _, origin := range configuration.HTTP.AllowedOrigins {
		parsed, err := url.ParseRequestURI(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("http.allowed_origins contains invalid origin %q", origin)
		}
	}
	return nil
}

func (configuration Config) validateDatabase() error {
	if strings.TrimSpace(configuration.Database.URL) == "" {
		return errors.New("database.url or TRACEPILOT_DATABASE_URL is required")
	}
	if configuration.Database.MaxConnections <= 0 || configuration.Database.MaxConnections > maxDatabaseConnections {
		return fmt.Errorf("database.max_connections must be between 1 and %d", maxDatabaseConnections)
	}
	if configuration.Database.MinConnections < 0 {
		return errors.New("database.min_connections must not be negative")
	}
	if configuration.Database.MinConnections > configuration.Database.MaxConnections {
		return errors.New("database.min_connections must not exceed max_connections")
	}
	if int32(configuration.Ingestion.Workers+2) > configuration.Database.MaxConnections {
		return errors.New("database.max_connections must reserve at least one connection per ingestion worker plus two")
	}
	if configuration.Database.MaxConnectionLifetime <= 0 {
		return errors.New("database.max_connection_lifetime must be positive")
	}
	if configuration.Database.MaxConnectionIdleTime <= 0 {
		return errors.New("database.max_connection_idle_time must be positive")
	}
	if configuration.Database.HealthCheckPeriod <= 0 {
		return errors.New("database.health_check_period must be positive")
	}
	if configuration.Database.ReadinessTimeout <= 0 {
		return errors.New("database.readiness_timeout must be positive")
	}
	if strings.TrimSpace(configuration.Database.MigrationsPath) == "" {
		return errors.New("database.migrations_path must not be empty")
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
