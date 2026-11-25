package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
)

func TestCreateAdminMakesTheOnlySuperAdministratorIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("requires a migrated disposable PostgreSQL database")
	}
	t.Setenv("TRACEPILOT_APP_PRODUCTION", "false")
	ctx := context.Background()
	databaseURL := os.Getenv("TRACEPILOT_DATABASE_URL")
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(ctx) })
	schema := fmt.Sprintf("create_admin_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	endpoint, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := endpoint.Query()
	query.Set("search_path", schema)
	endpoint.RawQuery = query.Encode()
	t.Setenv("TRACEPILOT_DATABASE_URL", endpoint.String())
	if code := runMigrations([]string{"postgres", "up"}); code != 0 {
		t.Fatalf("migrate postgres up: exit %d", code)
	}

	connection, err := pgx.Connect(ctx, endpoint.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close(ctx) })
	const first, second = "operator@tracepilot.test", "second-operator@tracepilot.test"

	create := func(email, password string) int {
		return runCreateAdmin([]string{"--email", email, "--name", "Release Operator", "--password-stdin"}, strings.NewReader(password+"\n"))
	}
	if code := create(first, "operator-chosen-password"); code != 0 {
		t.Fatalf("first create-admin: exit %d", code)
	}
	var hash string
	var platformAdmin, temporary bool
	var audits int
	err = connection.QueryRow(ctx, `SELECT password_hash, platform_admin, must_change_password,
(SELECT count(*) FROM audit_events WHERE target_id=users.id::text AND action='user.created' AND actor_user_id IS NULL)
FROM users WHERE email=$1`, first).Scan(&hash, &platformAdmin, &temporary, &audits)
	if err != nil {
		t.Fatal(err)
	}
	if !platformAdmin || temporary || audits != 1 || !auth.VerifyPassword("operator-chosen-password", hash) {
		t.Fatal("administrator, credential or transactional audit not stored")
	}
	if code := create(second, "different-new-password"); code == 0 {
		t.Fatal("a second create-admin made another super administrator")
	}

	var admins, strays int
	if err := connection.QueryRow(ctx, "SELECT count(*) FILTER (WHERE platform_admin), count(*) FILTER (WHERE email = $1) FROM users", second).Scan(&admins, &strays); err != nil {
		t.Fatal(err)
	}
	if admins != 1 || strays != 0 {
		t.Fatalf("super administrators=%d, refused accounts stored=%d; want exactly one and none", admins, strays)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO users (id, email, display_name, password_hash, platform_admin)
VALUES (gen_random_uuid(), $1, 'Bypass', 'unused', true)`, second); !pgvalue.IsUniqueViolationOf(err, "users_single_platform_admin") {
		t.Fatalf("writing a second super administrator straight to the database: %v; the schema must refuse it", err)
	}
}
