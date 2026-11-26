package hybrid

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/clienttest"

	"github.com/jackc/pgx/v5"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/migrations"
)

func TestMigrateAllStopsAtFailedDatabaseWithoutRollbackIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("requires a migrated disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	cfg := config.Config{Database: config.DatabaseConfig{URL: os.Getenv("TRACEPILOT_DATABASE_URL")},
		ClickHouse: config.ClickHouseConfig{URL: server.URL, Database: "test", Username: "default",
			RequestTimeout: time.Second, MaxConnections: 2}}
	if err := MigrateAll(ctx, cfg); err == nil || !strings.Contains(err.Error(), "PostgreSQL was not rolled back") || requests.Load() == 0 {
		t.Fatalf("ClickHouse failure did not stop the workflow: requests=%d error=%v", requests.Load(), err)
	}
	state, err := migrations.Inspect(ctx, cfg.Database.URL)
	if err != nil || state.Dirty || state.Version != state.Target {
		t.Fatalf("ClickHouse failure changed PostgreSQL history: %+v, %v", state, err)
	}
	admin, err := pgx.Connect(ctx, cfg.Database.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	schema := fmt.Sprintf("migration_stop_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE")
	if _, err := admin.Exec(ctx, "CREATE TABLE "+quoted+".schema_migrations(version bigint, dirty boolean); INSERT INTO "+quoted+".schema_migrations VALUES (1, true)"); err != nil {
		t.Fatal(err)
	}
	endpoint, err := url.Parse(cfg.Database.URL)
	if err != nil {
		t.Fatal(err)
	}
	query := endpoint.Query()
	query.Set("search_path", schema)
	endpoint.RawQuery = query.Encode()
	cfg.Database.URL = endpoint.String()
	requests.Store(0)
	if err := MigrateAll(ctx, cfg); err == nil || requests.Load() != 0 {
		t.Fatalf("PostgreSQL failure reached ClickHouse: requests=%d error=%v", requests.Load(), err)
	}
}

func TestMigrateAllDoesNotRequireApplicationSecretsIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" || os.Getenv("TRACEPILOT_CLICKHOUSE_INTEGRATION_TEST") != "1" {
		t.Skip("requires migrated disposable PostgreSQL and ClickHouse databases")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cfg := config.Config{
		Database: config.DatabaseConfig{URL: os.Getenv("TRACEPILOT_DATABASE_URL"), MaxConnections: 2,
			ConnectTimeout: time.Second, MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute},
		ClickHouse: config.ClickHouseConfig{URL: os.Getenv("TRACEPILOT_CLICKHOUSE_TEST_URL"),
			Database: fmt.Sprintf("tracepilot_migration_secrets_%d", time.Now().UnixNano()),
			Username: "default", RequestTimeout: 5 * time.Second, MaxConnections: 2},
	}
	defer clienttest.DropDatabase(t, cfg.ClickHouse)
	// Migration readiness only needs database access, never application credentials.
	cfg.Notifications.EncryptionKey = "not-a-webhook-key"
	if err := MigrateAll(ctx, cfg); err != nil {
		t.Fatal(err)
	}
}
