package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func useEnvironmentFile(t *testing.T, contents string) {
	t.Helper()
	backend := filepath.Join(t.TempDir(), "backend")
	if err := os.Mkdir(backend, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backend, "go.mod"), []byte("module test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if contents != "" {
		if err := os.WriteFile(filepath.Join(backend, ".env"), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(backend)
}

func TestLoadResolvesEachPropertyByPrecedence(t *testing.T) {
	tests := []struct {
		name  string
		file  string
		env   map[string]string
		check func(Config) bool
	}{
		{
			name:  "defaults without any source",
			check: func(c Config) bool { return c.Queue.Workers == 2 && c.App.ShutdownTimeout == 15*time.Second },
		},
		{
			name:  "file variable",
			file:  "TRACEPILOT_QUEUE_WORKERS=6\n",
			check: func(c Config) bool { return c.Queue.Workers == 6 },
		},
		{
			name:  "process variable wins over file variable",
			file:  "TRACEPILOT_QUEUE_WORKERS=6\n",
			env:   map[string]string{"TRACEPILOT_QUEUE_WORKERS": "8"},
			check: func(c Config) bool { return c.Queue.Workers == 8 },
		},
		{
			name: "partial override keeps sibling defaults",
			env:  map[string]string{"TRACEPILOT_QUEUE_WORKERS": "16", "TRACEPILOT_DATABASE_MAX_CONNECTIONS": "40"},
			check: func(c Config) bool {
				return c.Queue.Workers == 16 && c.Queue.QueueSize == 4_096 && c.Queue.BatchSize == 256 && c.Queue.BatchTimeout == 5*time.Second
			},
		},
		{
			name:  "explicit false overrides a true default",
			env:   map[string]string{"TRACEPILOT_INGESTION_SCRUB_FREE_TEXT": "false"},
			check: func(c Config) bool { return !c.Ingestion.ScrubFreeText },
		},
		{
			name:  "process false overrides file true",
			file:  "TRACEPILOT_APP_PRODUCTION=true\n",
			env:   map[string]string{"TRACEPILOT_APP_PRODUCTION": "false"},
			check: func(c Config) bool { return !c.App.Production },
		},
		{
			name:  "explicit info overrides a file debug level",
			file:  "TRACEPILOT_OBSERVABILITY_LOG_LEVEL=debug\n",
			env:   map[string]string{"TRACEPILOT_OBSERVABILITY_LOG_LEVEL": "info"},
			check: func(c Config) bool { return c.Observability.LogLevel == slog.LevelInfo },
		},
		{
			name:  "explicit zero overrides a default",
			env:   map[string]string{"TRACEPILOT_INGESTION_AUTH_FAILURE_LIMIT": "0", "TRACEPILOT_STORAGE_RETENTION_MAX_AGE": "0s"},
			check: func(c Config) bool { return c.Ingestion.AuthFailureLimit == 0 && c.Storage.Retention.MaxAge == 0 },
		},
		{
			name: "explicit empty values clear strings and lists",
			env:  map[string]string{"TRACEPILOT_AUTH_COOKIE_SAME_SITE": "", "TRACEPILOT_HTTP_ALLOWED_ORIGINS": ""},
			check: func(c Config) bool {
				return c.Auth.CookieSameSite == "" && c.HTTP.AllowedOrigins != nil && len(c.HTTP.AllowedOrigins) == 0
			},
		},
		{
			name: "nested groups map to path names",
			env: map[string]string{
				"TRACEPILOT_APP_SHUTDOWN_TIMEOUT":         "20s",
				"TRACEPILOT_APP_TLS_CERTIFICATE_FILE":     "tls.crt",
				"TRACEPILOT_APP_TLS_PRIVATE_KEY_FILE":     "tls.key",
				"TRACEPILOT_STORAGE_RETENTION_BATCH_SIZE": "500",
				"TRACEPILOT_REPLAY_TIMEOUT":               "3s",
				"TRACEPILOT_NOTIFICATIONS_POLL_INTERVAL":  "9s",
				"TRACEPILOT_QUEUE_QUEUE_SIZE":             "512",
				"TRACEPILOT_QUEUE_FLUSH_INTERVAL":         "50ms",
				"TRACEPILOT_CACHE_DRIVER":                 "noop",
			},
			check: func(c Config) bool {
				return c.App.ShutdownTimeout == 20*time.Second && c.App.TLS.CertificateFile == "tls.crt" &&
					c.App.TLS.PrivateKeyFile == "tls.key" && c.Storage.Retention.BatchSize == 500 &&
					c.Replay.Timeout == 3*time.Second && c.Notifications.PollInterval == 9*time.Second &&
					c.Queue.QueueSize == 512 && c.Queue.FlushInterval == 50*time.Millisecond &&
					c.Cache.Driver == CacheDriverNoop
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setTestEnvironment(t)
			useEnvironmentFile(t, test.file)
			for name, value := range test.env {
				t.Setenv(name, value)
			}
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if !test.check(cfg) {
				t.Fatalf("unexpected configuration: app=%+v queue=%+v observability=%+v", cfg.App, cfg.Queue, cfg.Observability)
			}
		})
	}
}

func TestLoadRejectsMalformedValuesWithoutEchoingThem(t *testing.T) {
	tests := []struct {
		variable, value, field string
	}{
		{"TRACEPILOT_QUEUE_WORKERS", "many-secret", "queue.workers"},
		{"TRACEPILOT_APP_PRODUCTION", "maybe-secret", "app.production"},
		{"TRACEPILOT_HTTP_REQUEST_TIMEOUT", "soon-secret", "http.request_timeout"},
		{"TRACEPILOT_OBSERVABILITY_LOG_LEVEL", "loud-secret", "observability.log_level"},
	}

	for _, test := range tests {
		t.Run(test.variable, func(t *testing.T) {
			setTestEnvironment(t)
			useEnvironmentFile(t, "")
			t.Setenv(test.variable, test.value)
			_, err := Load()
			if err == nil {
				t.Fatalf("%s=%q was accepted", test.variable, test.value)
			}
			message := err.Error()
			if !strings.Contains(message, test.field) || !strings.Contains(message, test.variable) {
				t.Fatalf("error %q must name %s and %s", message, test.field, test.variable)
			}
			if strings.Contains(message, "secret") {
				t.Fatalf("error %q exposed the configured value", message)
			}
		})
	}
}

func TestLoadAllowsMissingEnvironmentFile(t *testing.T) {
	setTestEnvironment(t)
	useEnvironmentFile(t, "")
	if _, err := Load(); err != nil {
		t.Fatalf("a missing backend/.env must fall back to defaults and the process: %v", err)
	}
}

func TestMigrationScopeIgnoresUnrelatedGroups(t *testing.T) {
	setTestEnvironment(t)
	useEnvironmentFile(t, "")
	t.Setenv("TRACEPILOT_QUEUE_WORKERS", "invalid")
	if _, err := LoadMigrations("postgres"); err != nil {
		t.Fatalf("migration loading parsed an unrelated group: %v", err)
	}
	t.Setenv("TRACEPILOT_APP_PRODUCTION", "true")
	if _, err := LoadMigrations("postgres"); err == nil {
		t.Fatal("the production flag was ignored by scoped loading")
	}
}

func TestEnvironmentExampleListsEveryVariable(t *testing.T) {
	clearConfigEnvironment(t)
	values, err := readEnvironmentFileAt(filepath.Join("..", "..", ".env.example"))
	if err != nil || values == nil {
		t.Fatalf("read .env.example: %v", err)
	}

	expected := map[string]bool{}
	for _, name := range configEnvironment() {
		expected[name] = true
	}
	for name := range expected {
		if _, listed := values[name]; !listed {
			t.Errorf(".env.example is missing %s", name)
		}
	}
	for name := range values {
		if strings.HasPrefix(name, environmentPrefix) && !expected[name] {
			t.Errorf(".env.example lists %s, which no setting reads", name)
		}
	}

	cfg := defaultConfig()
	if err := applyEnvironment(&cfg, values); err != nil {
		t.Fatalf(".env.example values do not parse: %v", err)
	}
}
