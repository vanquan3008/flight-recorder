package config

import (
	"log/slog"
)

// ObservabilityConfig groups settings for runtime logging, metrics and diagnostics.
type ObservabilityConfig struct {
	LogLevel slog.Level `config:"log_level"`
}

func (configuration Config) validateObservability(v *validator) {
	if configuration.HTTP.MetricsToken != "" && len(configuration.HTTP.MetricsToken) < 32 {
		v.add("http.metrics_token", "must contain at least 32 characters")
	}
}
