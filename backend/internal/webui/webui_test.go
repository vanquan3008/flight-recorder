package webui

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestHandlerServesConsoleAndAssetsOnly(t *testing.T) {
	handler := New()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `<div id="root"></div>`) {
		t.Fatalf("console = %d %q", response.Code, response.Body.String())
	}
	assetPath := regexp.MustCompile(`(?:src|href)="(/assets/[^"]+)"`).FindStringSubmatch(response.Body.String())
	if len(assetPath) != 2 {
		t.Fatalf("console has no built asset: %q", response.Body.String())
	}
	for _, test := range []struct {
		path   string
		status int
		cached bool
	}{
		{assetPath[1], http.StatusOK, true},
		{"/theme-init.js", http.StatusOK, false},
		{"/missing", http.StatusNotFound, false},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != test.status {
			t.Fatalf("%s = %d %q", test.path, response.Code, response.Body.String())
		}
		if response.Header().Get("Content-Security-Policy") == "" {
			t.Fatalf("%s has no CSP", test.path)
		}
		if test.path == "/theme-init.js" && response.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("theme bootstrap must be revalidated, got %q", response.Header().Get("Cache-Control"))
		}
		if test.cached && response.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
			t.Fatalf("%s has no immutable asset cache policy", test.path)
		}
	}
}
