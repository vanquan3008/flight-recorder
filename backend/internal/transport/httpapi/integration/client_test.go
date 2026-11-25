package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
)

const (
	refreshCookieName = "tp_refresh"
	refreshHeader     = "X-TracePilot-Refresh"
	refreshPath       = "/api/v1/auth"
)

type client struct {
	t       *testing.T
	handler http.Handler
	access  string
	refresh string
}

func (c *client) do(method, path, body, project string) (int, map[string]any) {
	c.t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if c.access != "" {
		request.Header.Set("Authorization", "Bearer "+c.access)
	}
	if project != "" {
		request.Header.Set(api.HeaderProjectID, project)
	}
	if strings.HasPrefix(path, refreshPath) {
		request.Header.Set(refreshHeader, "1")
		if c.refresh != "" {
			request.AddCookie(&http.Cookie{Name: refreshCookieName, Value: c.refresh})
		}
	}
	recorder := httptest.NewRecorder()
	c.handler.ServeHTTP(recorder, request)
	var decoded map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	if token, ok := decoded["access_token"].(string); ok {
		c.access = token
	}
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == refreshCookieName {
			c.refresh = cookie.Value
			if cookie.MaxAge < 0 {
				c.refresh = ""
			}
		}
	}
	return recorder.Code, decoded
}

func (c *client) signIn(email, password string) int {
	c.t.Helper()
	c.access, c.refresh = "", ""
	status, _ := c.do(http.MethodPost, "/api/v1/auth/login", `{"email":"`+email+`","password":"`+password+`"}`, "")
	return status
}

// createAccount has the super administrator make an account, then signs the person in with the
// temporary password and replaces it, as the console requires on first sign-in.
func (admin *client) createAccount(email, name, password string) (*client, string) {
	admin.t.Helper()
	status, body := admin.do(http.MethodPost, "/api/v1/users", `{"email":"`+email+`","display_name":"`+name+`","password":"a-temporary-passphrase"}`, "")
	userID, _ := nested(body, "user", "id").(string)
	if status != http.StatusCreated || userID == "" {
		admin.t.Fatalf("create %s: %d %v", email, status, body)
	}
	person := &client{t: admin.t, handler: admin.handler}
	if code := person.signIn(email, "a-temporary-passphrase"); code != http.StatusOK {
		admin.t.Fatalf("%s signs in with the temporary password: %d", email, code)
	}
	if status, _ = person.do(http.MethodGet, "/api/v1/projects", "", ""); status != http.StatusForbidden {
		admin.t.Fatalf("%s on a temporary password listed projects: status %d, want 403", email, status)
	}
	if status, body = person.do(http.MethodPost, "/api/v1/auth/password", `{"current_password":"a-temporary-passphrase","new_password":"`+password+`"}`, ""); status != http.StatusOK {
		admin.t.Fatalf("%s replaces the temporary password: %d %v", email, status, body)
	}
	return person, userID
}

func nested(body map[string]any, keys ...string) any {
	var current any = body
	for _, key := range keys {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[key]
	}
	return current
}

func mustDecode(_ int, body map[string]any) map[string]any { return body }

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}
