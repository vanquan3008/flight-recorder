package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

// testUser creates an account to act as; storage records who changed an incident or a setting.
func testUser(t testing.TB, database *Database) auth.User {
	t.Helper()
	email := fmt.Sprintf("actor-%d@example.test", time.Now().UnixNano())
	user, err := database.CreateUser(context.Background(), auth.CreateUserParams{Email: email, DisplayName: "Test actor", Password: "a-long-enough-passphrase"}, auth.Actor{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID) })
	return user
}
