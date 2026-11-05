package config

import (
	"time"

	notification "github.com/vanquan3008/tracepilot/internal/domain/notification"
)

// AppConfig contains application runtime and lifecycle configuration.
type AppConfig struct {
	Production      bool                `config:"production"`
	ShutdownTimeout time.Duration       `config:"shutdown_timeout"`
	TLS             TLSConfig           `config:"tls"`
	Replay          ReplayConfig        `config:"replay"`
	Notifications   NotificationsConfig `config:"notifications"`
}

// TLSConfig defines server TLS termination settings.
type TLSConfig struct {
	CertificateFile string `config:"certificate_file"`
	PrivateKeyFile  string `config:"private_key_file"`
}

// ReplayConfig controls active HTTP traffic reproduction for debugging.
type ReplayConfig struct {
	Enabled          bool          `config:"enabled"`
	AllowedHosts     []string      `config:"allowed_hosts"`
	AllowPrivate     bool          `config:"allow_private"`
	Timeout          time.Duration `config:"timeout"`
	MaxRequestBytes  int64         `config:"max_request_bytes"`
	MaxResponseBytes int64         `config:"max_response_bytes"`
	MaxConcurrent    int           `config:"max_concurrent"`
}

// NotificationsConfig configures incident delivery webhooks and background workers.
type NotificationsConfig struct {
	Enabled        bool          `config:"enabled"`
	EncryptionKey  string        `config:"encryption_key" env:"TRACEPILOT_WEBHOOK_ENCRYPTION_KEY"`
	PollInterval   time.Duration `config:"poll_interval" env:"TRACEPILOT_WEBHOOK_POLL_INTERVAL"`
	Workers        int           `config:"workers" env:"TRACEPILOT_WEBHOOK_WORKERS"`
	RequestTimeout time.Duration `config:"request_timeout" env:"TRACEPILOT_WEBHOOK_REQUEST_TIMEOUT"`
}

func (configuration Config) validateApp(validator *validator) {
	if (configuration.TLS.CertificateFile == "") != (configuration.TLS.PrivateKeyFile == "") {
		validator.add("tls", "certificate_file and private_key_file must be set together")
	}
	if configuration.Replay.Enabled && len(configuration.Replay.AllowedHosts) == 0 {
		validator.add("replay.allowed_hosts", "required when replay is enabled")
	}
	configuration.validateReplayLimits(validator)
	if configuration.ShutdownTimeout <= 0 {
		validator.add("shutdown_timeout", "must be positive")
	}
	configuration.validateNotifications(validator)
}

func (configuration Config) validateNotifications(validator *validator) {
	if !configuration.Notifications.Enabled {
		return
	}
	if _, err := notification.DecodeEncryptionKey(configuration.Notifications.EncryptionKey); err != nil {
		validator.add("notifications.encryption_key", err.Error())
	}
	if configuration.Notifications.PollInterval <= 0 {
		validator.add("notifications.poll_interval", "must be positive")
	}
	if configuration.Notifications.Workers <= 0 || configuration.Notifications.Workers > 64 {
		validator.add("notifications.workers", "must be between 1 and 64")
	}
	if configuration.Notifications.RequestTimeout <= 0 || configuration.Notifications.RequestTimeout > 5*time.Minute {
		validator.add("notifications.request_timeout", "must be between 1 and 5 minutes")
	}
}

func (configuration Config) validateReplayLimits(validator *validator) {
	if configuration.Replay.Timeout <= 0 {
		validator.add("replay.timeout", "must be positive")
	}
	if configuration.Replay.MaxRequestBytes <= 0 || configuration.Replay.MaxRequestBytes > 4<<20 {
		validator.add("replay.max_request_bytes", "must be between 1 and 4194304")
	}
	if configuration.Replay.MaxResponseBytes <= 0 || configuration.Replay.MaxResponseBytes > 4<<20 {
		validator.add("replay.max_response_bytes", "must be between 1 and 4194304")
	}
	if configuration.Replay.MaxConcurrent <= 0 || configuration.Replay.MaxConcurrent > 16 {
		validator.add("replay.max_concurrent", "must be between 1 and 16")
	}
}
