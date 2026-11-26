package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vanquan3008/tracepilot/internal/config"
)

func TestSessionParametersFollowConfiguration(t *testing.T) {
	connection, err := pgxpool.ParseConfig("postgres://u:p@localhost/db?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	applySessionParameters(connection.ConnConfig, config.DatabaseConfig{
		StatementTimeout: 30 * time.Second, LockTimeout: 1500 * time.Millisecond, IdleInTransactionTimeout: time.Minute,
	})
	want := map[string]string{
		"application_name": "tracepilot", "statement_timeout": "30000", "lock_timeout": "1500", "idle_in_transaction_session_timeout": "60000",
		"jit": "off",
	}
	for name, value := range want {
		if got := connection.ConnConfig.RuntimeParams[name]; got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
	}
}

func TestSessionParametersKeepServerDefaultsAtZeroAndURLWins(t *testing.T) {
	connection, err := pgxpool.ParseConfig("postgres://u:p@localhost/db?statement_timeout=5000&application_name=reporting&jit=on")
	if err != nil {
		t.Fatal(err)
	}
	applySessionParameters(connection.ConnConfig, config.DatabaseConfig{StatementTimeout: 30 * time.Second})
	params := connection.ConnConfig.RuntimeParams
	if params["statement_timeout"] != "5000" || params["application_name"] != "reporting" || params["jit"] != "on" {
		t.Fatalf("the URL did not win: %v", params)
	}
	for _, name := range []string{"lock_timeout", "idle_in_transaction_session_timeout"} {
		if _, set := params[name]; set {
			t.Errorf("%s was set although its limit is 0", name)
		}
	}
}

func TestPooledConnectionsCarryTheSessionLimitsIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	database, openError := Open(context.Background(), config.DatabaseConfig{
		URL: os.Getenv("TRACEPILOT_DATABASE_URL"), MaxConnections: 2, MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute,
		HealthCheckPeriod: time.Minute, StatementTimeout: 7 * time.Second, LockTimeout: 3 * time.Second, IdleInTransactionTimeout: 11 * time.Second,
	})
	if openError != nil {
		t.Fatal(openError)
	}
	defer database.Close()
	for name, want := range map[string]string{
		"statement_timeout": "7s", "lock_timeout": "3s", "idle_in_transaction_session_timeout": "11s", "application_name": "tracepilot",
		"jit": "off",
	} {
		var got string
		if scanError := database.pool.QueryRow(context.Background(), "SELECT current_setting($1)", name).Scan(&got); scanError != nil {
			t.Fatal(scanError)
		}
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	var sleepError = database.pool.QueryRow(context.Background(), "SELECT pg_sleep(9)").Scan(new(any))
	if sleepError == nil {
		t.Fatal("a query running past statement_timeout was not cancelled")
	}
}
