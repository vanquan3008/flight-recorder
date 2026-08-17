package config

import (
	"log/slog"
	"time"
)

func defaultConfig() Config {
	return Config{
		Version: CurrentVersion,
		HTTP: HTTPConfig{
			Address:                ":8080",
			ReadHeaderTimeout:      5 * time.Second,
			ReadTimeout:            15 * time.Second,
			WriteTimeout:           30 * time.Second,
			IdleTimeout:            60 * time.Second,
			MaxHeaderBytes:         1 << 20,
			ManagementMaxBodyBytes: 1 << 20,
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
		},
		Database: DatabaseConfig{
			MaxConnections:        20,
			MinConnections:        2,
			MaxConnectionLifetime: time.Hour,
			MaxConnectionIdleTime: 30 * time.Minute,
			HealthCheckPeriod:     time.Minute,
			ReadinessTimeout:      2 * time.Second,
			MigrationsPath:        "./migrations",
		},
		ShutdownTimeout: 15 * time.Second,
		LogLevel:        slog.LevelInfo,
	}
}
