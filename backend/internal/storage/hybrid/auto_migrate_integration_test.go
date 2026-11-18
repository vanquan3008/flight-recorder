package hybrid

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/migrations"
)

func TestAutoMigrationInitializesBothDatabasesOnlyWhenEnabledIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" || os.Getenv("TRACEPILOT_CLICKHOUSE_INTEGRATION_TEST") != "1" {
		t.Skip("requires disposable PostgreSQL and ClickHouse databases")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, os.Getenv("TRACEPILOT_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	name := fmt.Sprintf("tracepilot_auto_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), "DROP DATABASE "+quoted+" WITH (FORCE)")
	endpoint, err := url.Parse(os.Getenv("TRACEPILOT_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	endpoint.Path = "/" + name
	cfg := config.Config{Database: config.DatabaseConfig{URL: endpoint.String(), MaxConnections: 2,
		ConnectTimeout: time.Second, MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute},
		ClickHouse: config.ClickHouseConfig{URL: os.Getenv("TRACEPILOT_CLICKHOUSE_TEST_URL"), Database: name,
			Username: "default", RequestTimeout: 5 * time.Second, MaxConnections: 2}}
	defer dropClickHouseTestDatabase(t, cfg.ClickHouse)
	store, err := Open(ctx, cfg)
	if store != nil {
		store.Close()
		t.Fatal("uninitialized storage was accepted with auto migration disabled")
	}
	if err == nil {
		t.Fatal("missing schema did not block startup")
	}
	state, err := migrations.Inspect(ctx, cfg.Database.URL)
	if err != nil || state.Version != 0 {
		t.Fatalf("disabled startup changed PostgreSQL: %+v, %v", state, err)
	}
	logs, err := clickhouse.New(cfg.ClickHouse)
	if err != nil {
		t.Fatal(err)
	}
	defer logs.Close()
	chState, err := logs.MigrationStatus(ctx)
	if err != nil || chState.Version != 0 {
		t.Fatalf("disabled startup changed ClickHouse: %+v, %v", chState, err)
	}
	cfg.Database.AutoMigrate = true
	store, err = Open(ctx, cfg)
	if store != nil {
		store.Close()
		t.Fatal("PostgreSQL-only auto migration also initialized ClickHouse")
	}
	if err == nil {
		t.Fatal("missing ClickHouse schema did not block startup")
	}
	state, err = migrations.Inspect(ctx, cfg.Database.URL)
	if err != nil || state.Version != state.Target || state.Dirty {
		t.Fatalf("PostgreSQL-only auto migration: %+v, %v", state, err)
	}
	chState, err = logs.MigrationStatus(ctx)
	if err != nil || chState.Version != 0 {
		t.Fatalf("PostgreSQL flag changed ClickHouse history: %+v, %v", chState, err)
	}
	cfg.Database.AutoMigrate = false
	cfg.ClickHouse.AutoMigrate = true
	for range 2 {
		store, err := Open(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		readyErr := store.Ready(ctx)
		store.Close()
		if readyErr != nil {
			t.Fatal(readyErr)
		}
	}
	state, err = migrations.Inspect(ctx, cfg.Database.URL)
	if err != nil || state.Version != state.Target || state.Dirty {
		t.Fatalf("PostgreSQL auto migration: %+v, %v", state, err)
	}
	chState, err = logs.MigrationStatus(ctx)
	if err != nil || chState.Version != chState.Target || chState.Dirty {
		t.Fatalf("ClickHouse auto migration: %+v, %v", chState, err)
	}
	connection, err := pgx.Connect(ctx, cfg.Database.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(context.Background())
	if _, err := connection.Exec(ctx, "UPDATE schema_migrations SET dirty = true"); err != nil {
		t.Fatal(err)
	}
	cfg.Database.AutoMigrate = true
	store, err = Open(ctx, cfg)
	if store != nil {
		store.Close()
		t.Fatal("dirty schema allowed startup")
	}
	if err == nil || !strings.Contains(err.Error(), "automatic PostgreSQL migration") {
		t.Fatalf("migration failure did not block startup: %v", err)
	}
}

func dropClickHouseTestDatabase(t *testing.T, cfg config.ClickHouseConfig) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, cfg.URL+"/?query="+url.QueryEscape("DROP DATABASE IF EXISTS `"+cfg.Database+"`"), nil)
	if err != nil {
		t.Error(err)
		return
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Error(err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Error("ClickHouse test cleanup failed", response.StatusCode)
	}
}
