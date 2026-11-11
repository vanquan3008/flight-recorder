package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/migrations"
)

func TestSeedCreatesOnlySuperAdminAndPreservesAccountOnRerunIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("requires a disposable PostgreSQL server with CREATE DATABASE permission")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	endpoint, err := url.Parse(os.Getenv("TRACEPILOT_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.Connect(ctx, endpoint.String())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	name := fmt.Sprintf("tracepilot_seed_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+quoted+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	}()
	endpoint.Path = "/" + name
	if _, err := migrations.Run(ctx, endpoint.String(), "up", 0); err != nil {
		t.Fatal(err)
	}
	connection, err := pgx.Connect(ctx, endpoint.String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(context.Background())
	t.Chdir(t.TempDir())
	t.Setenv("TRACEPILOT_DATABASE_URL", endpoint.String())
	t.Setenv("TRACEPILOT_CLICKHOUSE_URL", "http://127.0.0.1:8123")
	t.Setenv("TRACEPILOT_PRODUCTION", "false")
	t.Setenv("TRACEPILOT_APP_PRODUCTION", "false")
	if code := runSeed(nil); code != 0 {
		t.Fatalf("seed exit = %d, want 0", code)
	}
	var userID, email, passwordHash string
	var platformAdmin bool
	if err := connection.QueryRow(ctx, "SELECT id::text, email, password_hash, platform_admin FROM users").
		Scan(&userID, &email, &passwordHash, &platformAdmin); err != nil {
		t.Fatal(err)
	}
	if email != adminEmail || !platformAdmin || !auth.VerifyPassword("admin-password-123", passwordHash) {
		t.Fatal("seed did not create the documented super admin credentials")
	}
	if _, err := connection.Exec(ctx, "UPDATE users SET display_name = 'Existing admin' WHERE id = $1", userID); err != nil {
		t.Fatal(err)
	}
	if code := runSeed(nil); code != 0 {
		t.Fatalf("seed rerun exit = %d, want 0", code)
	}
	var displayName, storedHash string
	if err := connection.QueryRow(ctx, "SELECT display_name, password_hash FROM users WHERE id = $1", userID).
		Scan(&displayName, &storedHash); err != nil {
		t.Fatal(err)
	}
	if displayName != "Existing admin" || storedHash != passwordHash {
		t.Fatal("seed rerun overwrote the existing account")
	}
	for table, want := range map[string]int{
		"users": 1, "projects": 0, "environments": 0, "project_members": 0, "ingest_tokens": 0,
		"services": 0, "events": 0, "spans": 0, "logs": 0, "metric_data_points": 0, "incidents": 0,
	} {
		var count int
		if err := connection.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Errorf("%s rows = %d, want %d", table, count, want)
		}
	}
}
