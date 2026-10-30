package config

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var configEnvironment = []string{
	"TRACEPILOT_CONFIG_FILE",
	"TRACEPILOT_HTTP_ADDRESS",
	"TRACEPILOT_HTTP_ALLOWED_ORIGINS",
	"TRACEPILOT_HTTP_TRUSTED_PROXIES",
	"TRACEPILOT_HTTP_MAX_CONCURRENT_QUERIES",
	"TRACEPILOT_HTTP_REQUEST_TIMEOUT",
	"TRACEPILOT_HTTP_READ_HEADER_TIMEOUT",
	"TRACEPILOT_HTTP_READ_TIMEOUT",
	"TRACEPILOT_HTTP_WRITE_TIMEOUT",
	"TRACEPILOT_HTTP_IDLE_TIMEOUT",
	"TRACEPILOT_HTTP_MAX_HEADER_BYTES",
	"TRACEPILOT_HTTP_MANAGEMENT_MAX_BODY_BYTES",
	"TRACEPILOT_HTTP_RATE_LIMIT_REQUESTS",
	"TRACEPILOT_HTTP_RATE_LIMIT_WINDOW",
	"TRACEPILOT_HTTP_RATE_LIMIT_AUTH_REQUESTS",
	"TRACEPILOT_HTTP_RATE_LIMIT_REPLAY_REQUESTS",
	"TRACEPILOT_INGESTION_GRPC_ADDRESS",
	"TRACEPILOT_INGESTION_HTTP_ADDRESS",
	"TRACEPILOT_INGESTION_PROJECT_ID",
	"TRACEPILOT_INGESTION_PROJECT_SLUG",
	"TRACEPILOT_INGESTION_PROJECT_NAME",
	"TRACEPILOT_INGESTION_ENVIRONMENT_ID",
	"TRACEPILOT_INGESTION_ENVIRONMENT_NAME",
	"TRACEPILOT_INGESTION_MAX_REQUEST_BYTES",
	"TRACEPILOT_INGESTION_MAX_RECORDS_PER_REQUEST",
	"TRACEPILOT_INGESTION_MAX_ATTRIBUTES_PER_RECORD",
	"TRACEPILOT_INGESTION_MAX_ATTRIBUTE_VALUE_BYTES",
	"TRACEPILOT_INGESTION_MAX_ATTRIBUTE_DEPTH",
	"TRACEPILOT_INGESTION_QUEUE_SIZE",
	"TRACEPILOT_INGESTION_QUEUE_MAX_BYTES",
	"TRACEPILOT_INGESTION_ENQUEUE_TIMEOUT",
	"TRACEPILOT_INGESTION_BATCH_SIZE",
	"TRACEPILOT_INGESTION_BATCH_TIMEOUT",
	"TRACEPILOT_INGESTION_FLUSH_INTERVAL",
	"TRACEPILOT_INGESTION_WORKERS",
	"TRACEPILOT_INGESTION_REDACTED_KEYS",
	"TRACEPILOT_SHUTDOWN_TIMEOUT",
	"TRACEPILOT_LOG_LEVEL",
	"TRACEPILOT_DATABASE_URL",
	"TRACEPILOT_DATABASE_MAX_CONNECTIONS",
	"TRACEPILOT_DATABASE_MIN_CONNECTIONS",
	"TRACEPILOT_DATABASE_MAX_CONNECTION_LIFETIME",
	"TRACEPILOT_DATABASE_MAX_CONNECTION_IDLE_TIME",
	"TRACEPILOT_DATABASE_HEALTH_CHECK_PERIOD",
	"TRACEPILOT_DATABASE_READINESS_TIMEOUT",
	"TRACEPILOT_MIGRATIONS_PATH",
	"TRACEPILOT_AUTH_BOOTSTRAP_API_KEY",
	"TRACEPILOT_PRODUCTION",
	"TRACEPILOT_TLS_CERTIFICATE_FILE",
	"TRACEPILOT_TLS_PRIVATE_KEY_FILE",
	"TRACEPILOT_RETENTION_MAX_AGE",
	"TRACEPILOT_RETENTION_INCIDENT_MAX_AGE",
	"TRACEPILOT_RETENTION_AUDIT_MAX_AGE",
	"TRACEPILOT_RETENTION_DELIVERY_MAX_AGE",
	"TRACEPILOT_RETENTION_CHECK_INTERVAL",
	"TRACEPILOT_RETENTION_BATCH_SIZE",
	"TRACEPILOT_REPLAY_ENABLED",
	"TRACEPILOT_NOTIFICATIONS_ENABLED",
	"TRACEPILOT_WEBHOOK_ENCRYPTION_KEY",
	"TRACEPILOT_WEBHOOK_POLL_INTERVAL",
	"TRACEPILOT_WEBHOOK_REQUEST_TIMEOUT",
	"TRACEPILOT_WEBHOOK_WORKERS",
	"TRACEPILOT_REPLAY_ALLOWED_HOSTS",
	"TRACEPILOT_REPLAY_ALLOW_PRIVATE",
	"TRACEPILOT_REPLAY_TIMEOUT",
	"TRACEPILOT_REPLAY_MAX_REQUEST_BYTES",
	"TRACEPILOT_REPLAY_MAX_RESPONSE_BYTES",
	"TRACEPILOT_REPLAY_MAX_CONCURRENT",
}

func TestNotificationsRequireValidEncryptionKeyWhenEnabled(t *testing.T) {
	configuration := defaultConfig()
	configuration.Database.URL = "postgres://test"
	configuration.Notifications.Enabled = true
	if err := configuration.Validate(); err == nil {
		t.Fatal("enabled notifications accepted a missing key")
	}
	configuration.Notifications.EncryptionKey = hex.EncodeToString(make([]byte, 32))
	if err := configuration.Validate(); err != nil {
		t.Fatalf("valid notification configuration rejected: %v", err)
	}
}

func TestLoadRateLimitEnvironmentOverrides(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("TRACEPILOT_DATABASE_URL", "postgres://environment")
	t.Setenv("TRACEPILOT_HTTP_RATE_LIMIT_REQUESTS", "7")
	t.Setenv("TRACEPILOT_HTTP_RATE_LIMIT_WINDOW", "30s")
	t.Setenv("TRACEPILOT_HTTP_RATE_LIMIT_AUTH_REQUESTS", "3")
	t.Setenv("TRACEPILOT_HTTP_RATE_LIMIT_REPLAY_REQUESTS", "2")
	configuration, loadError := Load()
	if loadError != nil {
		t.Fatal(loadError)
	}
	if configuration.HTTP.RateLimitRequests != 7 || configuration.HTTP.RateLimitWindow != 30*time.Second || configuration.HTTP.RateLimitAuthRequests != 3 || configuration.HTTP.RateLimitReplayRequests != 2 {
		t.Fatalf("rate limit environment overrides failed: %+v", configuration.HTTP)
	}
}

func TestLoadRejectsInvalidConnectionBounds(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("TRACEPILOT_DATABASE_URL", "postgres://test")
	t.Setenv("TRACEPILOT_DATABASE_MAX_CONNECTIONS", "2")
	t.Setenv("TRACEPILOT_DATABASE_MIN_CONNECTIONS", "3")
	if _, loadError := Load(); loadError == nil {
		t.Fatal("expected invalid connection bounds to fail")
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	clearConfigEnvironment(t)
	if _, loadError := Load(); loadError == nil {
		t.Fatal("expected missing database URL to fail")
	}
}

func TestLoadReadsRepositoryEnvironmentFile(t *testing.T) {
	clearConfigEnvironment(t)
	repositoryDirectory := t.TempDir()
	backendDirectory := filepath.Join(repositoryDirectory, "backend")
	if makeDirectoryError := os.Mkdir(backendDirectory, 0o700); makeDirectoryError != nil {
		t.Fatal(makeDirectoryError)
	}
	contents := "TRACEPILOT_DATABASE_URL=postgres://repository\n" +
		"TRACEPILOT_HTTP_READ_TIMEOUT=9s\n" +
		"TRACEPILOT_HTTP_ALLOWED_ORIGINS=https://one.example, https://two.example\n" +
		"TRACEPILOT_MIGRATIONS_PATH=/opt/tracepilot/migrations\n"
	if writeError := os.WriteFile(filepath.Join(repositoryDirectory, ".env"), []byte(contents), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	t.Chdir(backendDirectory)

	configuration, loadError := Load()
	if loadError != nil {
		t.Fatal(loadError)
	}
	if configuration.Database.URL != "postgres://repository" {
		t.Fatalf("database URL = %q", configuration.Database.URL)
	}
	if configuration.HTTP.ReadTimeout != 9*time.Second || len(configuration.HTTP.AllowedOrigins) != 2 {
		t.Fatalf("typed .env values were not loaded: %+v", configuration.HTTP)
	}
	if configuration.Database.MigrationsPath != "/opt/tracepilot/migrations" {
		t.Fatalf("legacy .env alias was not loaded: %q", configuration.Database.MigrationsPath)
	}
}

func TestLoadAllowsDisabledRetention(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("TRACEPILOT_DATABASE_URL", "postgres://test")
	t.Setenv("TRACEPILOT_RETENTION_MAX_AGE", "0s")
	configuration, loadError := Load()
	if loadError != nil {
		t.Fatal(loadError)
	}
	if configuration.Retention.MaxAge != 0 {
		t.Fatalf("retention max age = %s", configuration.Retention.MaxAge)
	}
}

func TestRetentionDefaultsKeepTelemetryShorterThanHistory(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("TRACEPILOT_DATABASE_URL", "postgres://test")
	configuration, loadError := Load()
	if loadError != nil {
		t.Fatal(loadError)
	}
	day := 24 * time.Hour
	got := configuration.Retention
	if got.MaxAge != 7*day || got.IncidentMaxAge != 90*day || got.AuditMaxAge != 180*day || got.DeliveryMaxAge != 30*day {
		t.Fatalf("retention defaults = %+v", got)
	}
}

func TestRetentionRejectsNegativeAges(t *testing.T) {
	for _, name := range []string{"MAX_AGE", "INCIDENT_MAX_AGE", "AUDIT_MAX_AGE", "DELIVERY_MAX_AGE"} {
		clearConfigEnvironment(t)
		t.Setenv("TRACEPILOT_DATABASE_URL", "postgres://test")
		t.Setenv("TRACEPILOT_RETENTION_"+name, "-1h")
		if _, loadError := Load(); loadError == nil {
			t.Fatalf("negative %s was accepted", name)
		}
	}
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range configEnvironment {
		originalValue, existed := os.LookupEnv(name)
		if unsetError := os.Unsetenv(name); unsetError != nil {
			t.Fatal(unsetError)
		}
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(name, originalValue)
				return
			}
			_ = os.Unsetenv(name)
		})
	}
}

func TestAPIQueryLimitsHaveSafeDefaultsAndRejectBadValues(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("TRACEPILOT_DATABASE_URL", "postgres://test")
	configuration, loadError := Load()
	if loadError != nil {
		t.Fatal(loadError)
	}
	if configuration.HTTP.MaxConcurrentQueries != 4 || configuration.HTTP.RequestTimeout != 20*time.Second || configuration.HTTP.RequestTimeout >= configuration.HTTP.WriteTimeout {
		t.Fatalf("defaults = %d slots, %s timeout, %s write timeout", configuration.HTTP.MaxConcurrentQueries, configuration.HTTP.RequestTimeout, configuration.HTTP.WriteTimeout)
	}
	for name, value := range map[string]string{
		"TRACEPILOT_HTTP_MAX_CONCURRENT_QUERIES": "0",
		"TRACEPILOT_HTTP_REQUEST_TIMEOUT":        "0s",
		"TRACEPILOT_HTTP_REQUEST_TIMEOUT ":       "45s", // longer than write_timeout: the client would see a closed connection
	} {
		clearConfigEnvironment(t)
		t.Setenv("TRACEPILOT_DATABASE_URL", "postgres://test")
		t.Setenv(strings.TrimSpace(name), value)
		if _, loadError := Load(); loadError == nil {
			t.Fatalf("%s=%s was accepted", name, value)
		}
	}
}

func TestLoadAppliesDefaultsThenEnvironment(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("TRACEPILOT_DATABASE_URL", "postgres://environment")
	t.Setenv("TRACEPILOT_HTTP_ADDRESS", ":9100")
	t.Setenv("TRACEPILOT_DATABASE_MAX_CONNECTIONS", "8")
	configuration, loadError := Load()
	if loadError != nil {
		t.Fatal(loadError)
	}
	if configuration.HTTP.Address != ":9100" || configuration.Database.MaxConnections != 8 {
		t.Fatalf("environment was not applied: %+v", configuration)
	}
	if configuration.HTTP.ReadTimeout != 15*time.Second {
		t.Fatalf("a setting left unset must keep its default: %+v", configuration.HTTP)
	}
}
