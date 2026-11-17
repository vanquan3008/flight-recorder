package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

func TestTokenRevocationMarkerAlwaysAdvancesIntegration(t *testing.T) {
	database := openIntegrationDatabase(t)
	t.Cleanup(database.Close)
	ctx := context.Background()
	user := testUser(t, database)
	// A slow application clock must not reuse a previous revocation version.
	future := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := database.pool.Exec(ctx, "UPDATE users SET tokens_valid_after=$1 WHERE id=$2", future, user.ID); err != nil {
		t.Fatal(err)
	}
	previous := future.UnixMicro()
	for range 2 {
		if err := database.SetPassword(ctx, user.ID, "a-replacement-passphrase", false, "", auth.Actor{}); err != nil {
			t.Fatal(err)
		}
		current, err := database.GetUser(ctx, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.TokenVersion() != previous+1 {
			t.Fatalf("version=%d want=%d", current.TokenVersion(), previous+1)
		}
		previous = current.TokenVersion()
	}
	disabled := true
	current, err := database.UpdateUser(ctx, user.ID, auth.UpdateUserParams{Disabled: &disabled}, auth.Actor{})
	if err != nil {
		t.Fatal(err)
	}
	if current.TokenVersion() != previous+1 {
		t.Fatal("disabling an account reused its old token version")
	}
	disabled = false
	enabled, err := database.UpdateUser(ctx, user.ID, auth.UpdateUserParams{Disabled: &disabled}, auth.Actor{})
	if err != nil {
		t.Fatal(err)
	}
	if enabled.TokenVersion() != current.TokenVersion() {
		t.Fatal("reenabling an account restored a revoked version")
	}
}
