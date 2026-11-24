package config

import "time"

const maxReplayBytes = 4 << 20

type ReplayConfig struct {
	Enabled          bool          `config:"enabled"`
	AllowedHosts     []string      `config:"allowed_hosts"`
	AllowPrivate     bool          `config:"allow_private"`
	Timeout          time.Duration `config:"timeout"`
	MaxRequestBytes  int64         `config:"max_request_bytes"`
	MaxResponseBytes int64         `config:"max_response_bytes"`
	MaxConcurrent    int           `config:"max_concurrent"`
}

func (configuration Config) validateReplay(validator *validator) {
	replay := configuration.Replay

	if replay.Enabled && len(replay.AllowedHosts) == 0 {
		validator.add("replay.allowed_hosts", "required when replay is enabled")
	}
	if replay.Timeout <= 0 {
		validator.add("replay.timeout", "must be positive")
	}
	if replay.MaxRequestBytes <= 0 || replay.MaxRequestBytes > maxReplayBytes {
		validator.add("replay.max_request_bytes", "must be between 1 and 4194304")
	}
	if replay.MaxResponseBytes <= 0 || replay.MaxResponseBytes > maxReplayBytes {
		validator.add("replay.max_response_bytes", "must be between 1 and 4194304")
	}
	if replay.MaxConcurrent <= 0 || replay.MaxConcurrent > 16 {
		validator.add("replay.max_concurrent", "must be between 1 and 16")
	}
}
