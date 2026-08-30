package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAPIKey(t *testing.T) {
	if key, err := resolveAPIKey("explicit", "", "environment"); err != nil || key != "explicit" {
		t.Fatalf("explicit key = %q, error = %v", key, err)
	}
	if key, err := resolveAPIKey("", "", " environment "); err != nil || key != "environment" {
		t.Fatalf("environment key = %q, error = %v", key, err)
	}
	path := filepath.Join(t.TempDir(), "api-key")
	if writeError := os.WriteFile(path, []byte(" file-secret\n"), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	if key, err := resolveAPIKey("", path, "environment"); err != nil || key != "file-secret" {
		t.Fatalf("file key = %q, error = %v", key, err)
	}
}

func TestResolveAPIKeyRejectsAmbiguousSources(t *testing.T) {
	if _, err := resolveAPIKey("explicit", "file", "environment"); err == nil {
		t.Fatal("expected explicit and file API key sources to conflict")
	}
}
