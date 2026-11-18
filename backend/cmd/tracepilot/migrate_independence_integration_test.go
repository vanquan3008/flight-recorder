package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
)

func TestPostgresMigrationCommandDoesNotRequireClickHouseIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("requires a disposable PostgreSQL database")
	}
	t.Setenv("TRACEPILOT_PRODUCTION", "false")
	t.Setenv("TRACEPILOT_CLICKHOUSE_URL", "invalid")
	t.Setenv("TRACEPILOT_CLICKHOUSE_REQUEST_TIMEOUT", "invalid")
	t.Setenv("TRACEPILOT_CLICKHOUSE_AUTO_MIGRATE", "invalid")
	t.Setenv("TRACEPILOT_RETENTION_MAX_AGE", "invalid")
	for _, operation := range []string{"up", "status", "version"} {
		if code := runMigrations([]string{"postgres", operation}); code != 0 {
			t.Fatalf("PostgreSQL %s depended on ClickHouse: exit %d", operation, code)
		}
	}
}

func TestClickHouseMigrationCommandDoesNotRequirePostgresIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_CLICKHOUSE_INTEGRATION_TEST") != "1" {
		t.Skip("requires a disposable ClickHouse instance")
	}
	cfg := config.ClickHouseConfig{URL: os.Getenv("TRACEPILOT_CLICKHOUSE_TEST_URL"),
		Database: fmt.Sprintf("tracepilot_independent_%d", time.Now().UnixNano())}
	defer dropClickHouseTestDatabase(t, cfg)
	t.Setenv("TRACEPILOT_PRODUCTION", "false")
	t.Setenv("TRACEPILOT_CLICKHOUSE_URL", cfg.URL)
	t.Setenv("TRACEPILOT_CLICKHOUSE_DATABASE", cfg.Database)
	t.Setenv("TRACEPILOT_CLICKHOUSE_USERNAME", "default")
	t.Setenv("TRACEPILOT_CLICKHOUSE_PASSWORD", "")
	t.Setenv("TRACEPILOT_RETENTION_MAX_AGE", "0s")
	t.Setenv("TRACEPILOT_DATABASE_URL", "invalid")
	t.Setenv("TRACEPILOT_DATABASE_MAX_CONNECTIONS", "invalid")
	t.Setenv("TRACEPILOT_DATABASE_AUTO_MIGRATE", "invalid")
	for _, operation := range []string{"up", "status"} {
		if code := runMigrations([]string{"clickhouse", operation}); code != 0 {
			t.Fatalf("ClickHouse %s depended on PostgreSQL: exit %d", operation, code)
		}
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
