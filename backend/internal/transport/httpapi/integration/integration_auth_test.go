package integration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
)

const integrationJWTSecret = "integration-jwt-secret-0123456789abcdef0123"

// newIntegrationAdmin returns an access token for the one super administrator. The schema allows
// only one, so every test shares it and creates it only when the database has none.
func newIntegrationAdmin(t testing.TB, database *postgres.Database) string {
	t.Helper()
	token, _, err := auth.IssueAccessToken([]byte(integrationJWTSecret), integrationAdminID(t, database), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func integrationAdminID(t testing.TB, database *postgres.Database) string {
	t.Helper()
	ctx := context.Background()
	user, err := database.CreateUser(ctx, auth.CreateUserParams{
		Email:       fmt.Sprintf("integration-admin-%d@example.test", time.Now().UnixNano()),
		DisplayName: "Integration admin", Password: "an-integration-passphrase", PlatformAdmin: true,
	}, auth.Actor{})
	if err == nil {
		return user.ID
	}
	if !errors.Is(err, auth.ErrPlatformAdminExists) {
		t.Fatal(err)
	}
	connection, err := pgx.Connect(ctx, os.Getenv("TRACEPILOT_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)
	var id string
	if err := connection.QueryRow(ctx, "SELECT id::text FROM users WHERE platform_admin").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// integrationAuth signs a request in and points it at a project.
func integrationAuth(request *http.Request, token, projectID string) {
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set(api.HeaderProjectID, projectID)
}
