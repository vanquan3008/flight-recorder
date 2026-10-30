package config

import (
	"log/slog"
	"time"
)

func defaultConfig() Config {
	return Config{
		HTTP: HTTPConfig{
			Address:                 ":8080",
			ReadHeaderTimeout:       5 * time.Second,
			ReadTimeout:             15 * time.Second,
			WriteTimeout:            30 * time.Second,
			IdleTimeout:             60 * time.Second,
			MaxHeaderBytes:          1 << 20,
			ManagementMaxBodyBytes:  1 << 20,
			MaxConcurrentQueries:    4,
			RequestTimeout:          20 * time.Second,
			RateLimitRequests:       120,
			RateLimitWindow:         time.Minute,
			RateLimitAuthRequests:   20,
			RateLimitReplayRequests: 10,
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
			QueueSize:              4_096,
			QueueMaxBytes:          64 << 20,
			EnqueueTimeout:         250 * time.Millisecond,
			BatchSize:              256,
			BatchTimeout:           5 * time.Second,
			FlushInterval:          200 * time.Millisecond,
			Workers:                2,
			AuthFailureLimit:       60,
			AuthFailureWindow:      time.Minute,
			ScrubFreeText:          true,
		},
		Database: DatabaseConfig{
			MaxConnections:        20,
			MinConnections:        2,
			MaxConnectionLifetime: time.Hour,
			MaxConnectionIdleTime: 30 * time.Minute,
			HealthCheckPeriod:     time.Minute,
			ReadinessTimeout:      2 * time.Second,
			MigrationsPath:        "./db/migrations",
			// Above http.request_timeout (20s) so the API answers 504 first and the
			// database timeout only catches work nobody is waiting for.
			StatementTimeout:         30 * time.Second,
			LockTimeout:              10 * time.Second,
			IdleInTransactionTimeout: 30 * time.Second,
		},
		Replay: ReplayConfig{
			Timeout:          5 * time.Second,
			MaxRequestBytes:  64 << 10,
			MaxResponseBytes: 64 << 10,
			MaxConcurrent:    2,
		},
		Retention: RetentionConfig{
			MaxAge:         7 * 24 * time.Hour,
			IncidentMaxAge: 90 * 24 * time.Hour,
			AuditMaxAge:    180 * 24 * time.Hour,
			DeliveryMaxAge: 30 * 24 * time.Hour,
			CheckInterval:  time.Hour,
			BatchSize:      10_000,
		},
		Notifications: NotificationsConfig{
			PollInterval:   5 * time.Second,
			Workers:        4,
			RequestTimeout: 10 * time.Second,
		},
		ShutdownTimeout: 15 * time.Second,
		LogLevel:        slog.LevelInfo,
	}
}
