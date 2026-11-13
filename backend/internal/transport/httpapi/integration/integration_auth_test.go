package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi"
)

const integrationJWTSecret = "integration-jwt-secret-0123456789abcdef0123"

// newIntegrationAdmin creates a super administrator and returns an access token for them.
func newIntegrationAdmin(t testing.TB, database *postgres.Database) string {
	t.Helper()
	email := fmt.Sprintf("integration-admin-%d@example.test", time.Now().UnixNano())
	user, err := database.CreateUser(context.Background(), auth.CreateUserParams{
		Email: email, DisplayName: "Integration admin", Password: "an-integration-passphrase", PlatformAdmin: true,
	}, auth.Actor{})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := auth.IssueAccessToken([]byte(integrationJWTSecret), user.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// integrationAuth signs a request in and points it at a project.
func integrationAuth(request *http.Request, token, projectID string) {
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set(httpapi.HeaderProjectID, projectID)
}
