package postgres

import "testing"

func TestPrincipalHasScope(t *testing.T) {
	principal := Principal{Scopes: []string{"read", "ingest"}}
	if !principal.HasScope("ingest") {
		t.Fatal("expected ingest scope to be granted")
	}
	if principal.HasScope("replay") {
		t.Fatal("unexpected replay scope")
	}
}
