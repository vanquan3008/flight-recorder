package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

func TestRevocationWithinTheSameSecondAppliesToRESTAndStreams(t *testing.T) {
	backend := newIdentityBackend(auth.RoleUser)
	server := newTestServer(nil)
	server.backend = backend
	at := time.Now().UTC().Truncate(time.Second)
	backend.user.TokensValidAfter = at.Add(100 * time.Millisecond)
	old, _, err := server.issueAccessToken(backend.user)
	if err != nil {
		t.Fatal(err)
	}
	backend.user.TokensValidAfter = at.Add(200 * time.Millisecond)
	next, _, err := server.issueAccessToken(backend.user)
	if err != nil {
		t.Fatal(err)
	}
	legacy, _, err := auth.IssueAccessToken(server.jwtSecret, backend.user.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token string
		allowed     bool
	}{
		{"revoked", old, false}, {"renewed", next, true}, {"legacy after revocation", legacy, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
			request.Header.Set("Authorization", "Bearer "+tc.token)
			response := httptest.NewRecorder()
			if _, allowed := server.authenticateScope(response, request, auth.ScopeSession); allowed != tc.allowed {
				t.Fatalf("REST allowed=%t want=%t body=%s", allowed, tc.allowed, response.Body)
			}
			claims, err := auth.VerifyJWT(server.jwtSecret, tc.token, auth.TokenAccess, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			principal := auth.Principal{UserID: backend.user.ID, ProjectID: testProjectIDForRoutes}
			if allowed := server.streamAccessAllowed(context.Background(), principal, claims); allowed != tc.allowed {
				t.Fatalf("stream allowed=%t want=%t", allowed, tc.allowed)
			}
		})
	}
}
