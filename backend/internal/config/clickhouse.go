package config

import (
	"net/url"
	"regexp"
	"strings"
	"time"
)

// An empty URL keeps the existing PostgreSQL log adapter.
type ClickHouseConfig struct {
	URL            string        `config:"url"`
	Database       string        `config:"database"`
	Username       string        `config:"username"`
	Password       string        `config:"password"`
	RequestTimeout time.Duration `config:"request_timeout"`
	MaxConnections int           `config:"max_connections"`
}

var clickHouseIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)

func (configuration Config) validateClickHouse(v *validator) {
	configuration.ClickHouse.validate(v, configuration.Production)
}

func (cfg ClickHouseConfig) Validate() error {
	var v validator
	cfg.validate(&v, false)
	return v.err()
}

func (cfg ClickHouseConfig) validate(v *validator, production bool) {
	if cfg.URL == "" {
		return
	}
	endpoint, err := url.Parse(cfg.URL)
	if err != nil || endpoint.Hostname() == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") ||
		endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || strings.Trim(endpoint.Path, "/") != "" {
		v.add("clickhouse.url", "must be an HTTP(S) origin without credentials, query or path")
	} else if production && endpoint.Scheme != "https" {
		v.add("clickhouse.url", "must use HTTPS in production")
	}
	if !clickHouseIdentifier.MatchString(cfg.Database) {
		v.add("clickhouse.database", "must be a simple database identifier")
	}
	if cfg.Username == "" || strings.ContainsAny(cfg.Username+cfg.Password, "\r\n") {
		v.add("clickhouse.credentials", "username is required and credentials must not contain newlines")
	}
	if cfg.RequestTimeout <= 0 || cfg.RequestTimeout > time.Minute {
		v.add("clickhouse.request_timeout", "must be positive and at most 1 minute")
	}
	if cfg.MaxConnections < 1 || cfg.MaxConnections > 64 {
		v.add("clickhouse.max_connections", "must be between 1 and 64")
	}
}
