package auth

import (
	"testing"
	"time"
)

func TestPrincipalHasScope(t *testing.T) {
	principal := Principal{Scopes: []string{"read", "ingest"}}
	if !principal.HasScope("ingest") || principal.HasScope("replay") {
		t.Fatal("scope membership is incorrect")
	}
}

func TestNormalizeCreateAPIKeyParams(t *testing.T) {
	now := time.Now().UTC()
	params, normalizeError := NormalizeCreateAPIKeyParams(CreateAPIKeyParams{
		Name: " release ", Scopes: []string{" write ", "read"}, ExpiresAt: pointerTo(now.Add(time.Hour)),
	}, now)
	if normalizeError != nil {
		t.Fatal(normalizeError)
	}
	if params.Name != "release" || len(params.Scopes) != 2 || params.Scopes[0] != "read" || params.Scopes[1] != "write" {
		t.Fatalf("unexpected normalized params: %#v", params)
	}
	for _, invalid := range []CreateAPIKeyParams{
		{Name: "", Scopes: []string{"read"}},
		{Name: "key"},
		{Name: "key", Scopes: []string{"read", "read"}},
		{Name: "key", Scopes: []string{"owner"}},
		{Name: "key", Scopes: []string{"read"}, ExpiresAt: pointerTo(now)},
	} {
		if _, normalizeError := NormalizeCreateAPIKeyParams(invalid, now); normalizeError == nil {
			t.Fatalf("NormalizeCreateAPIKeyParams(%#v) unexpectedly succeeded", invalid)
		}
	}
}

func pointerTo(value time.Time) *time.Time { return &value }
