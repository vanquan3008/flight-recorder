package config

import (
	"log/slog"
	"strings"
)

type ObservabilityConfig struct {
	LogLevel  slog.Level `config:"log_level"`
	LogFormat string     `config:"log_format"`
}

func (config Config) validateObservability(validator *validator) {
	obs := config.Observability

	switch obs.LogLevel {
	case slog.LevelDebug,
		slog.LevelInfo,
		slog.LevelWarn,
		slog.LevelError:
	default:
		validator.add(
			"observability.log_level",
			"must be DEBUG, INFO, WARN or ERROR",
		)
	}

	switch strings.ToLower(obs.LogFormat) {
	case "", "json", "text":
	default:
		validator.add(
			"observability.log_format",
			"must be json or text",
		)
	}
}
