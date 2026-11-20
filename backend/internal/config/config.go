package config

import (
	"log/slog"
	"time"
)

// Config represents the application runtime configuration.
type Config struct {
	App             AppConfig           `config:"app"`
	HTTP            HTTPConfig          `config:"http"`
	Database        DatabaseConfig      `config:"database"`
	ClickHouse      ClickHouseConfig    `config:"clickhouse"`
	Auth            AuthConfig          `config:"auth"`
	Cache           CacheConfig         `config:"cache"`
	Storage         StorageConfig       `config:"storage"`
	Queue           QueueConfig         `config:"queue"`
	Observability   ObservabilityConfig `config:"observability"`
	Production      bool                `config:"production"`
	ShutdownTimeout time.Duration       `config:"shutdown_timeout"`
	TLS             TLSConfig           `config:"tls"`
	Replay          ReplayConfig        `config:"replay"`
	Notifications   NotificationsConfig `config:"notifications"`
	Retention       RetentionConfig     `config:"retention"`
	Ingestion       IngestionConfig     `config:"ingestion"`
	LogLevel        slog.Level          `config:"log_level"`
	Seed            SeedConfig          `config:"seed"`
}

// Load initializes configuration using the following precedence:
//
// 1. Built-in defaults.
// 2. Environment file (.env / ../.env).
// 3. Process environment variables.
//
// Configuration is validated before being returned.
func Load() (Config, error) {
	return loadConfiguration(Config.Validate)
}

func loadConfiguration(validate func(Config) error, scopes ...string) (Config, error) {
	fileValues, err := readEnvironmentFile()
	if err != nil {
		return Config{}, err
	}

	cfg := defaultConfig()

	if err := applyEnvironment(&cfg, fileValues, scopes...); err != nil {
		return Config{}, err
	}

	// Keep unified app & runtime views aligned
	if !cfg.Production && cfg.App.Production {
		cfg.Production = cfg.App.Production
	} else if cfg.Production && !cfg.App.Production {
		cfg.App.Production = cfg.Production
	}
	if cfg.ShutdownTimeout == 0 && cfg.App.ShutdownTimeout > 0 {
		cfg.ShutdownTimeout = cfg.App.ShutdownTimeout
	} else if cfg.ShutdownTimeout > 0 && cfg.App.ShutdownTimeout == 0 {
		cfg.App.ShutdownTimeout = cfg.ShutdownTimeout
	}
	if cfg.TLS == (TLSConfig{}) && cfg.App.TLS != (TLSConfig{}) {
		cfg.TLS = cfg.App.TLS
	} else if cfg.App.TLS == (TLSConfig{}) && cfg.TLS != (TLSConfig{}) {
		cfg.App.TLS = cfg.TLS
	}
	if !cfg.Replay.Enabled && cfg.App.Replay.Enabled {
		cfg.Replay = cfg.App.Replay
	} else if cfg.Replay.Enabled && !cfg.App.Replay.Enabled {
		cfg.App.Replay = cfg.Replay
	}
	if !cfg.Notifications.Enabled && cfg.App.Notifications.Enabled {
		cfg.Notifications = cfg.App.Notifications
	} else if cfg.Notifications.Enabled && !cfg.App.Notifications.Enabled {
		cfg.App.Notifications = cfg.Notifications
	}

	// Keep unified observability & log level views aligned
	if cfg.LogLevel != 0 && cfg.Observability.LogLevel == 0 {
		cfg.Observability.LogLevel = cfg.LogLevel
	} else if cfg.Observability.LogLevel != 0 && cfg.LogLevel == 0 {
		cfg.LogLevel = cfg.Observability.LogLevel
	}

	// Keep unified retention & storage views aligned if not explicitly overridden
	if cfg.Retention != (RetentionConfig{}) && cfg.Storage.Retention == (RetentionConfig{}) {
		cfg.Storage.Retention = cfg.Retention
	} else if cfg.Storage.Retention != (RetentionConfig{}) && cfg.Retention == (RetentionConfig{}) {
		cfg.Retention = cfg.Storage.Retention
	}

	// Keep unified ingestion & queue views aligned
	if cfg.Queue == (QueueConfig{}) && cfg.Ingestion.QueueSize > 0 {
		cfg.Queue = QueueConfig{
			QueueSize:      cfg.Ingestion.QueueSize,
			QueueMaxBytes:  cfg.Ingestion.QueueMaxBytes,
			EnqueueTimeout: cfg.Ingestion.EnqueueTimeout,
			BatchSize:      cfg.Ingestion.BatchSize,
			BatchTimeout:   cfg.Ingestion.BatchTimeout,
			FlushInterval:  cfg.Ingestion.FlushInterval,
			Workers:        cfg.Ingestion.Workers,
		}
	} else if cfg.Queue.QueueSize > 0 && cfg.Ingestion.QueueSize == 0 {
		cfg.Ingestion.QueueSize = cfg.Queue.QueueSize
		cfg.Ingestion.QueueMaxBytes = cfg.Queue.QueueMaxBytes
		cfg.Ingestion.EnqueueTimeout = cfg.Queue.EnqueueTimeout
		cfg.Ingestion.BatchSize = cfg.Queue.BatchSize
		cfg.Ingestion.BatchTimeout = cfg.Queue.BatchTimeout
		cfg.Ingestion.FlushInterval = cfg.Queue.FlushInterval
		cfg.Ingestion.Workers = cfg.Queue.Workers
	}

	if err := validate(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
