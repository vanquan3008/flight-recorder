package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
)

func TestAccountsProjectsAndTokensIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("TRACEPILOT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TRACEPILOT_DATABASE_URL is required")
	}
	ctx := context.Background()
	database, openError := postgres.Open(ctx, config.DatabaseConfig{URL: databaseURL, MaxConnections: 4, MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute})
	if openError != nil {
		t.Fatal(openError)
	}
	t.Cleanup(database.Close)
	cleanup, poolError := pgxpool.New(ctx, databaseURL)
	if poolError != nil {
		t.Fatal(poolError)
	}
	t.Cleanup(cleanup.Close)

	run := strings.ReplaceAll(strings.ToLower(time.Now().UTC().Format("150405.000000000")), ".", "")
	projectID := telemetry.DeterministicID(time.Now().UTC(), "identity-project", run)
	ingestion := config.IngestionConfig{
		ProjectID: projectID, ProjectSlug: "identity-" + run, ProjectName: "Identity home",
		EnvironmentID: telemetry.DeterministicID(time.Now().UTC(), "identity-environment", run), EnvironmentName: "test",
	}
	if err := database.EnsureIngestionScope(ctx, ingestion); err != nil {
		t.Fatal(err)
	}
	adminEmail, aliceEmail, bobEmail := "admin-"+run+"@example.test", "alice-"+run+"@example.test", "bob-"+run+"@example.test"
	t.Cleanup(func() {
		for _, statement := range []string{
			"DELETE FROM projects WHERE slug LIKE 'acme-" + run + "%' OR id = '" + projectID + "'",
			"DELETE FROM users WHERE email IN ('" + adminEmail + "', '" + aliceEmail + "', '" + bobEmail + "')",
		} {
			if _, err := cleanup.Exec(ctx, statement); err != nil {
				t.Error(err)
			}
		}
	})
	server := New(config.Config{
		HTTP: config.HTTPConfig{ManagementMaxBodyBytes: 4096}, Ingestion: ingestion, Auth: config.AuthConfig{JWTSecret: integrationJWTSecret, AllowRegistration: true},
		Database: config.DatabaseConfig{ReadinessTimeout: time.Second}, ShutdownTimeout: time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), func(context.Context) error { return nil }, metrics.New("test", "test", "test"), database, nil)
	router := http.NewServeMux()
	server.registerAPIRoutes(router)

	// Anyone can register; nobody sees a project until they make one or are let in.
	hasUsers, _ := database.HasUsers(ctx)
	alice := &client{t: t, handler: router}
	status, body := alice.register(aliceEmail, "Alice", "a-long-enough-passphrase")
	if status != http.StatusCreated || alice.access == "" || alice.refresh == "" {
		t.Fatalf("register: %d %v", status, body)
	}
	aliceIsAdmin, _ := nested(body, "user", "platform_admin").(bool)
	if aliceIsAdmin == hasUsers {
		t.Fatalf("platform_admin = %t with existing accounts = %t; only the first account is a super administrator", aliceIsAdmin, hasUsers)
	}
	if status, _ = (&client{t: t, handler: router}).register(strings.ToUpper(aliceEmail), "Again", "a-long-enough-passphrase"); status != http.StatusConflict {
		t.Fatalf("registering an address twice: status %d, want 409", status)
	}
	if status, _ = (&client{t: t, handler: router}).register("bad-"+run+"@example.test", "Weak", "short"); status != http.StatusBadRequest {
		t.Fatalf("a weak password: status %d, want 400", status)
	}

	// A registered person creates a project for their product and becomes its owner.
	status, body = alice.do(http.MethodPost, "/api/v1/projects", `{"slug":"acme-`+run+`","name":"Acme","environment":"staging"}`, "")
	acme, _ := nested(body, "project", "id").(string)
	if status != http.StatusCreated || acme == "" {
		t.Fatalf("create project: %d %v", status, body)
	}
	if status, body = alice.do(http.MethodGet, "/api/v1/projects/"+acme, "", ""); status != http.StatusOK || nested(body, "role") != "owner" {
		t.Fatalf("get project: %d %v", status, body)
	}
	if status, _ = alice.do(http.MethodPost, "/api/v1/projects/"+acme+"/environments", `{"name":"canary"}`, ""); status != http.StatusCreated {
		t.Fatalf("create environment: status %d", status)
	}
	if status, _ = alice.do(http.MethodPost, "/api/v1/projects/"+acme+"/environments", `{"name":"canary"}`, ""); status != http.StatusConflict {
		t.Fatalf("duplicate environment: status %d, want 409", status)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
	request.Header.Set("Authorization", "Bearer "+alice.access)
	request.Header.Set(projectHeader, acme)
	request.Header.Set(environmentHeader, "no-such-environment")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("an environment the project lacks: status %d, want 400", recorder.Code)
	}

	// Another registered person sees nothing of it until the owner lets them in.
	bob := &client{t: t, handler: router}
	if status, body = bob.register(bobEmail, "Bob", "another-long-passphrase"); status != http.StatusCreated {
		t.Fatalf("register bob: %d %v", status, body)
	}
	bobID, _ := nested(body, "user", "id").(string)
	if status, _ = bob.do(http.MethodGet, "/api/v1/services", "", acme); status != http.StatusForbidden {
		t.Fatalf("a registered person who was not added: status %d, want 403", status)
	}
	if status, _ = alice.do(http.MethodPost, "/api/v1/projects/"+acme+"/members", `{"email":"nobody-`+run+`@example.test","role":"user"}`, ""); status != http.StatusNotFound {
		t.Fatalf("adding an address with no account: status %d, want 404", status)
	}
	if status, body = alice.do(http.MethodPost, "/api/v1/projects/"+acme+"/members", `{"email":"`+bobEmail+`","role":"user"}`, ""); status != http.StatusCreated {
		t.Fatalf("add member: %d %v", status, body)
	}
	if status, _ = alice.do(http.MethodPost, "/api/v1/projects/"+acme+"/members", `{"email":"`+bobEmail+`","role":"user"}`, ""); status != http.StatusConflict {
		t.Fatalf("adding twice: status %d, want 409", status)
	}
	if status, _ = bob.do(http.MethodGet, "/api/v1/services", "", acme); status != http.StatusOK {
		t.Fatalf("a user reads the project: status %d", status)
	}
	for _, denied := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/projects/" + acme + "/members", ""},
		{http.MethodPost, "/api/v1/projects/" + acme + "/ingest-tokens", `{"name":"x"}`},
		{http.MethodPost, "/api/v1/projects/" + acme + "/environments", `{"name":"x"}`},
		{http.MethodGet, "/api/v1/audit-events", ""},
		{http.MethodGet, "/api/v1/users", ""},
	} {
		if status, _ = bob.do(denied.method, denied.path, denied.body, acme); status != http.StatusForbidden {
			t.Errorf("a user calling %s %s: status %d, want 403", denied.method, denied.path, status)
		}
	}

	// A telemetry sender holds an ingest token: a JWT for this project, revocable.
	status, body = alice.do(http.MethodPost, "/api/v1/projects/"+acme+"/ingest-tokens", `{"name":"checkout"}`, "")
	jwt, _ := nested(body, "jwt").(string)
	tokenID, _ := nested(body, "token", "id").(string)
	if status != http.StatusCreated || jwt == "" || tokenID == "" {
		t.Fatalf("create ingest token: %d %v", status, body)
	}
	if status, _ = alice.do(http.MethodPost, "/api/v1/projects/"+acme+"/ingest-tokens", `{"name":"checkout"}`, ""); status != http.StatusConflict {
		t.Fatalf("duplicate token name: status %d, want 409", status)
	}
	principal, authError := auth.AuthenticateIngestToken(ctx, []byte(integrationJWTSecret), database, jwt, time.Now())
	if authError != nil || principal.ProjectID != acme || !principal.HasScope(auth.ScopeIngest) {
		t.Fatalf("principal = %+v, error = %v", principal, authError)
	}
	if status, body = alice.do(http.MethodGet, "/api/v1/projects/"+acme+"/ingest-tokens", "", ""); status != http.StatusOK || strings.Contains(string(mustJSON(body)), jwt) {
		t.Fatalf("listing must show the token without its secret: %d %v", status, body)
	}
	if status, _ = alice.do(http.MethodDelete, "/api/v1/projects/"+acme+"/ingest-tokens/"+tokenID, "", ""); status != http.StatusNoContent {
		t.Fatalf("revoke: status %d", status)
	}
	if _, authError = auth.AuthenticateIngestToken(ctx, []byte(integrationJWTSecret), database, jwt, time.Now()); authError != auth.ErrUnauthorized {
		t.Fatalf("a revoked token must stop working at once, got %v", authError)
	}

	// The audit trail names who changed access.
	if status, body = alice.do(http.MethodGet, "/api/v1/audit-events", "", acme); status != http.StatusOK || !strings.Contains(string(mustJSON(body)), "member.added") || !strings.Contains(string(mustJSON(body)), "ingest_token.created") {
		t.Fatalf("audit events: %d %v", status, body)
	}

	// Owners come in twos; a project is never left without one.
	if status, _ = alice.do(http.MethodPatch, "/api/v1/projects/"+acme+"/members/"+bobID, `{"role":"owner"}`, ""); status != http.StatusOK {
		t.Fatalf("promote: status %d", status)
	}
	if status, _ = bob.do(http.MethodGet, "/api/v1/projects/"+acme+"/members", "", ""); status != http.StatusOK {
		t.Fatalf("a promoted owner lists members: status %d", status)
	}
	aliceID, _ := nested(mustDecode(alice.do(http.MethodGet, "/api/v1/auth/me", "", "")), "user", "id").(string)
	if status, _ = alice.do(http.MethodDelete, "/api/v1/projects/"+acme+"/members/"+aliceID, "", ""); status != http.StatusNoContent {
		t.Fatalf("an owner leaving while another remains: status %d", status)
	}
	// A super administrator keeps access to every project, so only an ordinary account loses it by leaving.
	if status, _ = alice.do(http.MethodGet, "/api/v1/services", "", acme); !aliceIsAdmin && status != http.StatusForbidden {
		t.Fatalf("someone who left: status %d, want 403", status)
	}
	if status, body = bob.do(http.MethodDelete, "/api/v1/projects/"+acme+"/members/"+bobID, "", ""); status != http.StatusConflict || nested(body, "error", "code") != "LAST_OWNER" {
		t.Fatalf("the last owner leaving: %d %v", status, body)
	}

	// Refreshing replaces the refresh token; a used one is dead.
	used := alice.refresh
	if status, _ = alice.do(http.MethodPost, "/api/v1/auth/refresh", "", ""); status != http.StatusOK || alice.refresh == used || alice.refresh == "" {
		t.Fatalf("refresh: status %d, token replaced: %t", status, alice.refresh != used)
	}
	replay := &client{t: t, handler: router, refresh: used}
	if status, _ = replay.do(http.MethodPost, "/api/v1/auth/refresh", "", ""); status != http.StatusUnauthorized {
		t.Fatalf("replaying a used refresh token: status %d, want 401", status)
	}

	// A password change ends the access tokens issued before it and every other session, not this one.
	time.Sleep(1100 * time.Millisecond) // tokens carry whole seconds, so the change must land in a later one
	oldAccess := alice.access
	other := &client{t: t, handler: router}
	if code := other.signIn(aliceEmail, "a-long-enough-passphrase"); code != http.StatusOK {
		t.Fatalf("second sign-in: %d", code)
	}
	time.Sleep(1100 * time.Millisecond)
	if status, _ = alice.do(http.MethodPost, "/api/v1/auth/password", `{"current_password":"a-long-enough-passphrase","new_password":"a-brand-new-passphrase"}`, ""); status != http.StatusOK {
		t.Fatalf("change password: status %d", status)
	}
	if alice.access == oldAccess {
		t.Fatal("changing the password must answer with a new access token")
	}
	staleRequest := &client{t: t, handler: router, access: oldAccess}
	if status, _ = staleRequest.do(http.MethodGet, "/api/v1/auth/me", "", ""); status != http.StatusUnauthorized {
		t.Fatalf("an access token from before the password change: status %d, want 401", status)
	}
	if status, _ = other.do(http.MethodPost, "/api/v1/auth/refresh", "", ""); status != http.StatusUnauthorized {
		t.Fatalf("another browser's session after a password change: status %d, want 401", status)
	}
	if status, _ = alice.do(http.MethodGet, "/api/v1/auth/me", "", ""); status != http.StatusOK {
		t.Fatalf("the browser that changed it stays signed in: status %d", status)
	}
	if code := (&client{t: t, handler: router}).signIn(aliceEmail, "a-long-enough-passphrase"); code != http.StatusUnauthorized {
		t.Fatalf("the old password: status %d, want 401", code)
	}

	// A super administrator can end an account's access at once.
	if _, err := database.CreateUser(ctx, auth.CreateUserParams{Email: adminEmail, DisplayName: "Admin", Password: "an-administrator-passphrase", PlatformAdmin: true}, auth.Actor{}); err != nil {
		t.Fatal(err)
	}
	admin := &client{t: t, handler: router}
	if code := admin.signIn(adminEmail, "an-administrator-passphrase"); code != http.StatusOK {
		t.Fatalf("admin sign in: %d", code)
	}
	if status, _ = admin.do(http.MethodPatch, "/api/v1/users/"+bobID, `{"disabled":true}`, ""); status != http.StatusOK {
		t.Fatalf("disable: status %d", status)
	}
	if status, _ = bob.do(http.MethodGet, "/api/v1/services", "", acme); status != http.StatusUnauthorized {
		t.Fatalf("a disabled account's access token: status %d, want 401", status)
	}
	if status, _ = bob.do(http.MethodPost, "/api/v1/auth/refresh", "", ""); status != http.StatusUnauthorized {
		t.Fatalf("a disabled account refreshing: status %d, want 401", status)
	}

	// Signing out ends the refresh session on the server.
	refresh := alice.refresh
	if status, _ = alice.do(http.MethodPost, "/api/v1/auth/logout", "", ""); status != http.StatusNoContent {
		t.Fatalf("logout: status %d", status)
	}
	if status, _ = (&client{t: t, handler: router, refresh: refresh}).do(http.MethodPost, "/api/v1/auth/refresh", "", ""); status != http.StatusUnauthorized {
		t.Fatalf("a signed-out session: status %d, want 401", status)
	}
}
