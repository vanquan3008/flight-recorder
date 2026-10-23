// Package capture tests request sanitization.
package capture

import (
	"net/http"
	"testing"
)

func TestSanitizeRedactsJSONAndDropsCredentials(t *testing.T) {
	request, requestError := Sanitize(http.MethodPost, "https://staging.test", http.Header{"Content-Type": []string{"application/json"}, "Authorization": []string{"Bearer secret"}}, []byte(`{"token":"secret","safe":"value"}`), 1024)
	if requestError != nil {
		t.Fatal(requestError)
	}
	if string(request.Body) != `{"safe":"value","token":"[REDACTED]"}` || request.Header.Get("Authorization") != "" {
		t.Fatalf("unsafe capture: %+v", request)
	}
}
