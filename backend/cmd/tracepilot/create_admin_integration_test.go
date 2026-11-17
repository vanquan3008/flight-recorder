package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/auth"
)

func TestCreateAdminDoesNotOverwriteAndAuditsIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("requires a migrated disposable PostgreSQL database")
	}
	t.Setenv("TRACEPILOT_PRODUCTION", "false")
	t.Setenv("TRACEPILOT_APP_PRODUCTION", "false")
	ctx := context.Background()
	connection, err := pgx.Connect(ctx, os.Getenv("TRACEPILOT_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close(ctx) })
	email := fmt.Sprintf("operator-%d@tracepilot.test", time.Now().UnixNano())
	t.Cleanup(func() {
		for _, statement := range []string{
			"DELETE FROM audit_events WHERE target_id IN (SELECT id::text FROM users WHERE email=$1)",
			"DELETE FROM users WHERE email=$1",
		} {
			if _, err := connection.Exec(ctx, statement, email); err != nil {
				t.Error(err)
			}
		}
	})
	args := []string{"--email", email, "--name", "Release Operator", "--password-stdin"}
	if code := runCreateAdmin(args, strings.NewReader("operator-chosen-password\n")); code != 0 {
		t.Fatalf("create-admin exit=%d", code)
	}
	if code := runCreateAdmin(args, strings.NewReader("different-new-password\n")); code == 0 {
		t.Fatal("duplicate administrator was overwritten")
	}
	var hash string
	var admin, temporary bool
	var audits int
	err = connection.QueryRow(ctx, `SELECT password_hash, platform_admin, must_change_password,
(SELECT count(*) FROM audit_events WHERE target_id=users.id::text AND action='user.created' AND actor_user_id IS NULL)
FROM users WHERE email=$1`, email).Scan(&hash, &admin, &temporary, &audits)
	if err != nil {
		t.Fatal(err)
	}
	if !admin || temporary || audits != 1 || !auth.VerifyPassword("operator-chosen-password", hash) {
		t.Fatal("administrator, original credential or transactional audit not preserved")
	}
}
