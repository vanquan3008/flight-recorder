package config

import (
	"log/slog"
	"time"
)

func defaultConfig() Config {
	return Config{
		App: AppConfig{
			ShutdownTimeout: 15 * time.Second,
		},
		HTTP: HTTPConfig{
			Address:                 ":8080",
			ReadHeaderTimeout:       5 * time.Second,
			ReadTimeout:             15 * time.Second,
			WriteTimeout:            30 * time.Second,
			IdleTimeout:             60 * time.Second,
			MaxHeaderBytes:          1 << 20,
			ManagementMaxBodyBytes:  1 << 20,
			MaxConcurrentQueries:    4,
			MaxConcurrentLogQueries: 8,
			RequestTimeout:          20 * time.Second,
			RateLimitRequests:       120,
			RateLimitWindow:         time.Minute,
			RateLimitAuthRequests:   20,
			RateLimitReplayRequests: 10,
		},
		Database: DatabaseConfig{
			MaxConnections:            20,
			MinConnections:            2,
			ConnectTimeout:            DefaultDatabaseConnectTimeout,
			MaxConnectionLifetime:     time.Hour,
			MaxConnectionIdleTime:     30 * time.Minute,
			HealthCheckPeriod:         time.Minute,
			ReadinessTimeout:          2 * time.Second,
			StatementTimeout:          30 * time.Second,
			LockTimeout:               10 * time.Second,
			IdleInTransactionTimeout:  30 * time.Second,
			MigrationLockTimeout:      10 * time.Second,
			MigrationStatementTimeout: 5 * time.Minute,
			PartitionWindowDays:       10,
			PartitionCheckInterval:    time.Hour,
		},
		ClickHouse: ClickHouseConfig{
			Database:       "tracepilot",
			Username:       "default",
			RequestTimeout: 10 * time.Second,
			MaxConnections: 8,
		},
		Auth: AuthConfig{
			AllowRegistration: true,
			AccessTokenTTL:    15 * time.Minute,
			RefreshTokenTTL:   30 * 24 * time.Hour,
			CookieSameSite:    "strict",
			Issuer:            "tracepilot",
			Audience:          "tracepilot",
		},
		Cache: CacheConfig{
			Driver:        CacheDriverAuto,
			DefaultTTL:    60 * time.Second,
			DefaultLimit:  4096,
			TokenTTL:      30 * time.Second,
			TokenLimit:    4096,
			ScopeTTL:      30 * time.Second,
			ScopeLimit:    1024,
			IdentityTTL:   30 * time.Second,
			IdentityLimit: 4096,
		},
		Storage: StorageConfig{
			Retention: RetentionConfig{
				MaxAge:         7 * 24 * time.Hour,
				IncidentMaxAge: 90 * 24 * time.Hour,
				AuditMaxAge:    180 * 24 * time.Hour,
				DeliveryMaxAge: 30 * 24 * time.Hour,
				CheckInterval:  time.Hour,
				BatchSize:      10_000,
			},
		},
		Queue: QueueConfig{
			QueueSize:      4_096,
			QueueMaxBytes:  64 << 20,
			EnqueueTimeout: 250 * time.Millisecond,
			BatchSize:      256,
			BatchTimeout:   5 * time.Second,
			FlushInterval:  200 * time.Millisecond,
			Workers:        2,
		},
		Ingestion: IngestionConfig{
			GRPCAddress:            ":4317",
			HTTPAddress:            ":4318",
			ProjectID:              "01900000-0000-7000-8000-000000000001",
			ProjectSlug:            "default",
			ProjectName:            "Default Project",
			EnvironmentID:          "01900000-0000-7000-8000-000000000002",
			EnvironmentName:        "development",
			MaxRequestBytes:        4 << 20,
			MaxRecordsPerRequest:   10_000,
			MaxConcurrentRequests:  32,
			MaxAttributesPerRecord: 128,
			MaxAttributeValueBytes: 4 << 10,
			MaxAttributeDepth:      4,
			AuthFailureLimit:       60,
			AuthFailureWindow:      time.Minute,
			ScrubFreeText:          true,
		},
		Observability: ObservabilityConfig{
			LogLevel: slog.LevelInfo,
		},
		Replay: ReplayConfig{
			Timeout:          5 * time.Second,
			MaxRequestBytes:  64 << 10,
			MaxResponseBytes: 64 << 10,
			MaxConcurrent:    2,
		},
		Seed: SeedConfig{AdminName: "Admin"},
		Notifications: NotificationsConfig{
			PollInterval:   5 * time.Second,
			Workers:        4,
			RequestTimeout: 10 * time.Second,
		},
	}
}
