package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentFileDoesNotMutateProcessAndCanReload(t *testing.T) {
	clearConfigEnvironment(t)
	t.Chdir(t.TempDir())
	for _, databaseURL := range []string{"postgres://first", "postgres://second"} {
		if err := os.WriteFile(".env", []byte("TRACEPILOT_DATABASE_URL="+databaseURL+"\nTRACEPILOT_HTTP_ADDRESS=:9200\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("TRACEPILOT_HTTP_ADDRESS", ":9300")
		configuration, err := Load("")
		if err != nil {
			t.Fatal(err)
		}
		if configuration.Database.URL != databaseURL || configuration.HTTP.Address != ":9300" {
			t.Fatal(".env reload or process environment precedence failed")
		}
		if _, exists := os.LookupEnv("TRACEPILOT_DATABASE_URL"); exists {
			t.Fatal("loading .env changed the process environment")
		}
	}
}

func TestNativeBackendCommandReadsRepositoryRootEnvironment(t *testing.T) {
	clearConfigEnvironment(t)
	root := t.TempDir()
	backend := filepath.Join(root, "backend")
	if err := os.Mkdir(backend, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TRACEPILOT_DATABASE_URL=postgres://root-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{root, backend} {
		t.Chdir(directory)
		configuration, err := Load("")
		if err != nil || configuration.Database.URL != "postgres://root-file" {
			t.Fatalf("load root .env from %s: configuration=%+v error=%v", directory, configuration, err)
		}
	}
}

func TestInvalidLocalEnvironmentFileDoesNotFallBackOrExposeContents(t *testing.T) {
	clearConfigEnvironment(t)
	root := t.TempDir()
	backend := filepath.Join(root, "backend")
	if err := os.Mkdir(backend, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TRACEPILOT_DATABASE_URL=postgres://fallback\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(backend)
	if err := os.WriteFile(".env", []byte("SENSITIVE='private-value-without-closing-quote"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load("")
	if err == nil {
		t.Fatal("invalid .env silently fell back to parent file")
	}
	if strings.Contains(err.Error(), "private-value") {
		t.Fatal("configuration error exposed file contents")
	}
}
