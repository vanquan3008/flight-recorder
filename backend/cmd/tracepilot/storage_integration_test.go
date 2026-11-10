package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/config"
)

func TestStorageStartupFailureClosesPostgresPoolIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("requires a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("TRACEPILOT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TRACEPILOT_DATABASE_URL must point to a disposable test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	observer, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close(context.Background())
	name := fmt.Sprintf("tracepilot_startup_%d", time.Now().UnixNano())
	endpoint, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	values := endpoint.Query()
	values.Set("application_name", name)
	endpoint.RawQuery = values.Encode()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	cfg := config.Config{
		Database: config.DatabaseConfig{URL: endpoint.String(), MaxConnections: 4, MinConnections: 1,
			ConnectTimeout: time.Second, MaxConnectionLifetime: time.Minute,
			MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute},
		ClickHouse: config.ClickHouseConfig{URL: server.URL, Database: "test", Username: "default",
			RequestTimeout: time.Second, MaxConnections: 2},
	}
	backend, err := openStorageBackend(ctx, cfg)
	if err == nil || backend != nil {
		if backend != nil {
			backend.Close()
		}
		t.Fatal("startup accepted unavailable ClickHouse")
	}
	var connections int
	if err := observer.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE application_name = $1", name).Scan(&connections); err != nil {
		t.Fatal(err)
	}
	if connections != 0 {
		t.Fatalf("failed startup leaked %d PostgreSQL connections", connections)
	}
}
