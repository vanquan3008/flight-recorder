package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	notification "github.com/vanquan3008/tracepilot/internal/domain/notification"
)

func (configuration Config) validateNotifications() error {
	if !configuration.Notifications.Enabled {
		return nil
	}
	if _, err := notification.DecodeEncryptionKey(configuration.Notifications.EncryptionKey); err != nil {
		return fmt.Errorf("notifications.encryption_key: %w", err)
	}
	if configuration.Notifications.PollInterval <= 0 {
		return errors.New("notifications.poll_interval must be positive")
	}
	if configuration.Notifications.Workers <= 0 || configuration.Notifications.Workers > 64 {
		return errors.New("notifications.workers must be between 1 and 64")
	}
	if configuration.Notifications.RequestTimeout <= 0 || configuration.Notifications.RequestTimeout > 5*time.Minute {
		return errors.New("notifications.request_timeout must be between 1 and 5 minutes")
	}
	return nil
}

func (configuration Config) validateRuntime() error {
	if configuration.Production && configuration.Auth.BootstrapAPIKey == "" {
		return errors.New("auth.bootstrap_api_key is required in production")
	}
	if configuration.Production && configuration.TLS.CertificateFile == "" {
		return errors.New("tls.certificate_file and tls.private_key_file are required in production")
	}
	if configuration.Production {
		if err := validateProductionDatabaseTLS(configuration.Database.URL); err != nil {
			return err
		}
	}
	if configuration.Auth.BootstrapAPIKey != "" && len(configuration.Auth.BootstrapAPIKey) < 32 {
		return errors.New("auth.bootstrap_api_key must contain at least 32 characters")
	}
	if (configuration.TLS.CertificateFile == "") != (configuration.TLS.PrivateKeyFile == "") {
		return errors.New("tls.certificate_file and tls.private_key_file must be set together")
	}
	if configuration.Replay.Enabled && len(configuration.Replay.AllowedHosts) == 0 {
		return errors.New("replay.allowed_hosts is required when replay is enabled")
	}
	if err := configuration.validateReplayLimits(); err != nil {
		return err
	}
	if err := configuration.validateRetentionLimits(); err != nil {
		return err
	}
	if configuration.ShutdownTimeout <= 0 {
		return errors.New("shutdown_timeout must be positive")
	}
	return nil
}

func (configuration Config) validateReplayLimits() error {
	if configuration.Replay.Timeout <= 0 {
		return errors.New("replay.timeout must be positive")
	}
	if configuration.Replay.MaxRequestBytes <= 0 || configuration.Replay.MaxRequestBytes > 4<<20 {
		return fmt.Errorf("replay.max_request_bytes must be between 1 and %d", 4<<20)
	}
	if configuration.Replay.MaxResponseBytes <= 0 || configuration.Replay.MaxResponseBytes > 4<<20 {
		return fmt.Errorf("replay.max_response_bytes must be between 1 and %d", 4<<20)
	}
	if configuration.Replay.MaxConcurrent <= 0 || configuration.Replay.MaxConcurrent > 16 {
		return errors.New("replay.max_concurrent must be between 1 and 16")
	}
	return nil
}

func (configuration Config) validateRetentionLimits() error {
	for name, age := range map[string]time.Duration{
		"max_age": configuration.Retention.MaxAge, "incident_max_age": configuration.Retention.IncidentMaxAge,
		"audit_max_age": configuration.Retention.AuditMaxAge, "delivery_max_age": configuration.Retention.DeliveryMaxAge,
	} {
		if age < 0 {
			return fmt.Errorf("retention.%s must not be negative", name)
		}
	}
	if configuration.Retention.CheckInterval <= 0 {
		return errors.New("retention.check_interval must be positive")
	}
	if configuration.Retention.BatchSize <= 0 || configuration.Retention.BatchSize > maxIngestionBatchSize {
		return fmt.Errorf("retention.batch_size must be between 1 and %d", maxIngestionBatchSize)
	}
	return nil
}

// validateProductionDatabaseTLS rejects database URLs that can fall back to
// plaintext. libpq defaults to sslmode=prefer, so a missing sslmode is rejected
// too. Unix sockets never leave the host and are exempt. require encrypts but
// does not authenticate the server; verify-full is the recommended mode.
func validateProductionDatabaseTLS(databaseURL string) error {
	parsed, parseError := url.Parse(databaseURL)
	if parseError != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return errors.New("database.url must be a postgres:// URL in production")
	}
	query := parsed.Query()
	if strings.HasPrefix(parsed.Hostname(), "/") || strings.HasPrefix(query.Get("host"), "/") {
		return nil
	}
	switch query.Get("sslmode") {
	case "require", "verify-ca", "verify-full":
		return nil
	}
	return errors.New("database.url must set sslmode to require, verify-ca, or verify-full in production")
}
