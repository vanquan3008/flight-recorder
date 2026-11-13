package auth

import "testing"

func TestPrincipalHasScope(t *testing.T) {
	principal := Principal{Scopes: []string{"read", "ingest"}}
	if !principal.HasScope("ingest") || principal.HasScope("replay") {
		t.Fatal("scope membership is incorrect")
	}
}
