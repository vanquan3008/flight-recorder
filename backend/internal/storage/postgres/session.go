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
	// JIT compiles a plan once its estimated cost passes jit_above_cost, and the
	// planner's estimates for span aggregates are often far above the real work.
	// Measured on 150k spans in a 24 hour trace list: 2.7 s with JIT, of which
	// 2.1 s was LLVM compilation, against 1.0 s without it. These short queries
	// never recover that compile time.
	parameters := map[string]string{"application_name": applicationName, "jit": "off"}
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
