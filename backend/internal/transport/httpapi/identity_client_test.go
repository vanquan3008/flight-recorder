package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// client is one browser: it keeps the access token in memory and the refresh cookie, as the console does.
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
		request.Header.Set(projectHeader, project)
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

func (c *client) register(email, name, password string) (int, map[string]any) {
	c.t.Helper()
	return c.do(http.MethodPost, "/api/v1/auth/register", `{"email":"`+email+`","display_name":"`+name+`","password":"`+password+`"}`, "")
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
