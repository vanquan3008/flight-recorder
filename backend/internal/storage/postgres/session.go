package postgres

import (
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/config"
)

// applicationName labels pooled connections in pg_stat_activity and logs.
const applicationName = "tracepilot"

// applySessionParameters sets per-connection limits and the application name.
// Parameters already present on the database URL are left alone so an operator
// can override one without a code or config change.
func applySessionParameters(connection *pgx.ConnConfig, configuration config.DatabaseConfig) {
	if connection.RuntimeParams == nil {
		connection.RuntimeParams = make(map[string]string)
	}
	parameters := map[string]string{"application_name": applicationName}
	for name, limit := range map[string]time.Duration{
		"statement_timeout":                   configuration.StatementTimeout,
		"lock_timeout":                        configuration.LockTimeout,
		"idle_in_transaction_session_timeout": configuration.IdleInTransactionTimeout,
	} {
		if limit > 0 {
			parameters[name] = strconv.FormatInt(limit.Milliseconds(), 10)
		}
	}
	for name, value := range parameters {
		if _, fromURL := connection.RuntimeParams[name]; !fromURL {
			connection.RuntimeParams[name] = value
		}
	}
}
