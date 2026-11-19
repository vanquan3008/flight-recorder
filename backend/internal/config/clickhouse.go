package config

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var clickHouseIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)

type LogPartition string

const (
	LogPartitionDaily   LogPartition = "daily"
	LogPartitionMonthly LogPartition = "monthly"
	LogPartitionYearly  LogPartition = "yearly"
	LogPartitionWeekly  LogPartition = "weekly"
)

// PartitionExpr returns the ClickHouse PARTITION BY expression for this granularity.
func (p LogPartition) PartitionExpr() string {
	switch p {
	case LogPartitionDaily:
		return "toYYYYMMDD(occurred_at, 'UTC')"
	case LogPartitionMonthly:
		return "toYYYYMM(occurred_at, 'UTC')"
	case LogPartitionYearly:
		return "toYear(occurred_at, 'UTC')"
	default: // weekly
		return "toStartOfWeek(occurred_at, 1, 'UTC')"
	}
}

// ClickHouseConfig configures ClickHouse HTTP(S) connectivity.
type ClickHouseConfig struct {
	URL            string        `config:"url"`
	AutoMigrate    bool          `config:"auto_migrate"`
	Database       string        `config:"database"`
	Username       string        `config:"username"`
	Password       string        `config:"password"`
	RequestTimeout time.Duration `config:"request_timeout"`
	MaxConnections int           `config:"max_connections"`
	// LogPartition controls the PARTITION BY granularity of the log table.
	// Accepted values: daily, monthly, yearly, weekly (default).
	LogPartition LogPartition `config:"log_partition"`
}

func (configuration Config) validateClickHouse(v *validator) {
	configuration.ClickHouse.validate(v, configuration.Production)
}

func (c ClickHouseConfig) Validate() error {
	return c.ValidateForEnvironment(false)
}

func (c ClickHouseConfig) ValidateForEnvironment(production bool) error {
	var v validator
	c.validate(&v, production)
	return v.err()
}

func (c ClickHouseConfig) validate(v *validator, production bool) {
	// ClickHouse URL is required — the application does not fall back to
	// PostgreSQL for telemetry storage.
	if c.URL == "" {
		v.add("clickhouse.url", "ClickHouse URL is required for telemetry storage")
		return
	}

	c.validateURL(v, production)
	c.validateDatabase(v)
	c.validateCredentials(v)
	c.validateConnection(v)
	c.validateLogPartition(v)
}

func (c ClickHouseConfig) validateLogPartition(v *validator) {
	switch c.LogPartition {
	case "", LogPartitionWeekly, LogPartitionDaily, LogPartitionMonthly, LogPartitionYearly:
		// valid
	default:
		v.add("clickhouse.log_partition", "must be one of: daily, monthly, yearly, weekly")
	}
}

func (c ClickHouseConfig) validateURL(v *validator, production bool) {
	endpoint, err := url.Parse(c.URL)
	if err != nil {
		v.add("clickhouse.url", "invalid URL")
		return
	}

	if (endpoint.Scheme != "http" && endpoint.Scheme != "https") ||
		endpoint.Hostname() == "" ||
		strings.ContainsAny(endpoint.Hostname(), " \t\r\n") ||
		endpoint.User != nil ||
		endpoint.Opaque != "" ||
		endpoint.RawQuery != "" ||
		endpoint.ForceQuery ||
		strings.Contains(c.URL, "#") ||
		(endpoint.Path != "" && endpoint.Path != "/") {

		v.add("clickhouse.url", "must be an HTTP(S) origin without credentials, query, fragment or custom path")
		return
	}

	if strings.HasSuffix(endpoint.Host, ":") {
		v.add("clickhouse.url", "invalid port")
		return
	}

	if port := endpoint.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			v.add("clickhouse.url", "port must be between 1 and 65535")
		}
	}

	if production && endpoint.Scheme != "https" {
		v.add("clickhouse.url", "HTTPS is required in production")
	}
}

func (c ClickHouseConfig) validateDatabase(v *validator) {
	if !clickHouseIdentifier.MatchString(c.Database) {
		v.add("clickhouse.database", "must be a valid database identifier")
	}
}

func (c ClickHouseConfig) validateCredentials(v *validator) {
	if c.Username == "" {
		v.add("clickhouse.username", "username is required")
	}
	if strings.ContainsAny(c.Username+c.Password, "\r\n") {
		v.add("clickhouse.credentials", "credentials must not contain newlines")
	}
}

func (c ClickHouseConfig) validateConnection(v *validator) {
	if c.RequestTimeout <= 0 || c.RequestTimeout > time.Minute {
		v.add("clickhouse.request_timeout", "must be positive and at most 1 minute")
	}
	if c.MaxConnections < 1 || c.MaxConnections > 64 {
		v.add("clickhouse.max_connections", "must be between 1 and 64")
	}
}
