package postgres

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/migrations"
)

func TestReadyValidatesSchemaThroughRuntimePoolIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("requires a disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("TRACEPILOT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TRACEPILOT_DATABASE_URL must point to a disposable test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	schema := fmt.Sprintf("readiness_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE")
	endpoint, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	values := endpoint.Query()
	values.Set("search_path", schema)
	endpoint.RawQuery = values.Encode()
	database, err := Open(ctx, config.DatabaseConfig{URL: endpoint.String(), MaxConnections: 2,
		ConnectTimeout: time.Second, MaxConnectionLifetime: time.Minute,
		MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Ready(ctx); err == nil || !strings.Contains(err.Error(), "run tracepilot migrate up") {
		t.Fatalf("missing schema did not give migration instructions: %v", err)
	}
	if _, err := database.pool.Exec(ctx, "CREATE TABLE schema_migrations (version bigint, dirty boolean)"); err != nil {
		t.Fatal(err)
	}
	if err := database.Ready(ctx); err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("empty schema version was accepted: %v", err)
	}
	expected, err := migrations.CurrentVersion()
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []struct {
		version          uint
		dirty, wantError bool
	}{
		{expected, false, false},
		{expected, true, true},
		{expected - 1, false, true},
	} {
		if _, err := database.pool.Exec(ctx, "DELETE FROM schema_migrations"); err != nil {
			t.Fatal(err)
		}
		if _, err := database.pool.Exec(ctx, "INSERT INTO schema_migrations VALUES ($1, $2)", state.version, state.dirty); err != nil {
			t.Fatal(err)
		}
		if err := database.Ready(ctx); (err != nil) != state.wantError {
			t.Errorf("version=%d dirty=%t: %v", state.version, state.dirty, err)
		}
	}
}
