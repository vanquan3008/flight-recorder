package config

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ClickHouseConfig configures the ClickHouse.
type ClickHouseConfig struct {
	URL            string        `config:"url"`
	Database       string        `config:"database"`
	Username       string        `config:"username"`
	Password       string        `config:"password"`
	RequestTimeout time.Duration `config:"request_timeout"`
	MaxConnections int           `config:"max_connections"`
}

var clickHouseIdentifier = regexp.MustCompile(
	`^[A-Za-z_][A-Za-z0-9_]{0,62}$`,
)

func (configuration Config) validateClickHouse(validator *validator) {
	configuration.ClickHouse.validate(
		validator,
		configuration.Production,
	)
}

func (configuration ClickHouseConfig) Validate() error {
	return configuration.ValidateForEnvironment(false)
}

func (configuration ClickHouseConfig) ValidateForEnvironment(production bool) error {
	var validator validator

	configuration.validate(&validator, production)

	return validator.err()
}

func (configuration ClickHouseConfig) validate(
	validator *validator,
	production bool,
) {
	if configuration.URL == "" {
		return
	}

	configuration.validateURL(validator, production)
	configuration.validateDatabase(validator)
	configuration.validateCredentials(validator)
	configuration.validateConnection(validator)
}

func (configuration ClickHouseConfig) validateURL(
	validator *validator,
	production bool,
) {
	endpoint, err := url.Parse(configuration.URL)

	if err != nil {
		validator.add("clickhouse.url", "invalid URL")
		return
	}

	if (endpoint.Scheme != "http" && endpoint.Scheme != "https") ||
		endpoint.Hostname() == "" ||
		endpoint.User != nil ||
		endpoint.Opaque != "" ||
		endpoint.ForceQuery ||
		endpoint.RawQuery != "" ||
		strings.Contains(configuration.URL, "#") ||
		(endpoint.Path != "" && endpoint.Path != "/") {

		validator.add(
			"clickhouse.url",
			"must be an HTTP(S) origin without credentials, query, fragment or custom path",
		)
		return
	}

	if strings.HasSuffix(endpoint.Host, ":") {
		validator.add("clickhouse.url", "invalid port")
		return
	}

	if port := endpoint.Port(); port != "" {
		portNumber, err := strconv.Atoi(port)

		if err != nil || portNumber < 1 || portNumber > 65535 {
			validator.add(
				"clickhouse.url",
				"port must be between 1 and 65535",
			)
		}
	}

	if production && endpoint.Scheme != "https" {
		validator.add(
			"clickhouse.url",
			"HTTPS is required in production",
		)
	}
}

func (configuration ClickHouseConfig) validateDatabase(
	validator *validator,
) {
	if !clickHouseIdentifier.MatchString(configuration.Database) {
		validator.add(
			"clickhouse.database",
			"must be a valid database identifier",
		)
	}
}

func (configuration ClickHouseConfig) validateCredentials(
	validator *validator,
) {
	if configuration.Username == "" {
		validator.add(
			"clickhouse.username",
			"username is required",
		)
	}

	if strings.ContainsAny(
		configuration.Username+configuration.Password,
		"\r\n",
	) {
		validator.add(
			"clickhouse.credentials",
			"credentials must not contain newlines",
		)
	}
}

func (configuration ClickHouseConfig) validateConnection(
	validator *validator,
) {
	if configuration.RequestTimeout <= 0 ||
		configuration.RequestTimeout > time.Minute {

		validator.add(
			"clickhouse.request_timeout",
			"must be positive and at most 1 minute",
		)
	}

	if configuration.MaxConnections < 1 ||
		configuration.MaxConnections > 64 {

		validator.add(
			"clickhouse.max_connections",
			"must be between 1 and 64",
		)
	}
}
