package config

import (
	"errors"
	"fmt"
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

func (configuration Config) validateSecurity() error {
	if !configuration.Security.Enabled {
		return nil
	}
	if strings.TrimSpace(configuration.Security.SyslogAddress) == "" {
		return errors.New("security.syslog_address is required when security is enabled")
	}
	if configuration.Security.SyslogAddress == configuration.HTTP.Address ||
		configuration.Security.SyslogAddress == configuration.Ingestion.GRPCAddress ||
		configuration.Security.SyslogAddress == configuration.Ingestion.HTTPAddress {
		return errors.New("security.syslog_address must be distinct from HTTP and OTLP addresses")
	}
	if configuration.Security.MaxLineBytes < 1024 || configuration.Security.MaxLineBytes > 1<<20 {
		return fmt.Errorf("security.max_line_bytes must be between 1024 and %d", 1<<20)
	}
	if configuration.Security.MaxConnections <= 0 || configuration.Security.MaxConnections > 10_000 {
		return errors.New("security.max_connections must be between 1 and 10000")
	}
	if configuration.Security.IdleTimeout <= 0 {
		return errors.New("security.idle_timeout must be positive")
	}
	if configuration.Security.DetectionInterval <= 0 {
		return errors.New("security.detection_interval must be positive")
	}
	if configuration.Production && !configuration.Security.SyslogTLS {
		return errors.New("production security syslog requires TLS (set security.syslog_tls: true)")
	}
	if configuration.Security.SyslogTLS && (configuration.TLS.CertificateFile == "" || configuration.TLS.PrivateKeyFile == "") {
		return errors.New("security.syslog_tls requires tls.certificate_file and tls.private_key_file")
	}
	if configuration.Production && strings.TrimSpace(configuration.Security.SyslogClientCAFile) == "" {
		return errors.New("production security syslog requires security.syslog_client_ca_file for mTLS client verification")
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
	if configuration.Retention.MaxAge < 0 {
		return errors.New("retention.max_age must not be negative")
	}
	if configuration.Retention.CheckInterval <= 0 {
		return errors.New("retention.check_interval must be positive")
	}
	if configuration.Retention.BatchSize <= 0 || configuration.Retention.BatchSize > maxIngestionBatchSize {
		return fmt.Errorf("retention.batch_size must be between 1 and %d", maxIngestionBatchSize)
	}
	return nil
}
