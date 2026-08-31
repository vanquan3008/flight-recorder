package postgres

import "testing"

func TestNormalizeAPIKeyScopes(t *testing.T) {
	scopes, normalizeError := NormalizeAPIKeyScopes([]string{" write ", "read"})
	if normalizeError != nil {
		t.Fatalf("NormalizeAPIKeyScopes() error = %v", normalizeError)
	}
	if len(scopes) != 2 || scopes[0] != "read" || scopes[1] != "write" {
		t.Fatalf("NormalizeAPIKeyScopes() = %#v", scopes)
	}
	for _, invalid := range [][]string{nil, {"read", "read"}, {"owner"}} {
		if _, normalizeError := NormalizeAPIKeyScopes(invalid); normalizeError == nil {
			t.Fatalf("NormalizeAPIKeyScopes(%#v) unexpectedly succeeded", invalid)
		}
	}
}
