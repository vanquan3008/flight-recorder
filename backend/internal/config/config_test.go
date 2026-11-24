package config

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNotificationsRequireValidEncryptionKeyWhenEnabled(t *testing.T) {
	configuration := validConfig()
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
	setTestEnvironment(t)
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

func TestLoadCachePoliciesFromEnvironment(t *testing.T) {
	setTestEnvironment(t)
	t.Setenv("TRACEPILOT_DATABASE_URL", "postgres://environment")
	t.Setenv("TRACEPILOT_CACHE_TOKEN_TTL", "45s")
	t.Setenv("TRACEPILOT_CACHE_TOKEN_LIMIT", "512")
	t.Setenv("TRACEPILOT_CACHE_SCOPE_TTL", "1m")
	t.Setenv("TRACEPILOT_CACHE_SCOPE_LIMIT", "128")
	t.Setenv("TRACEPILOT_CACHE_IDENTITY_TTL", "2m")
	t.Setenv("TRACEPILOT_CACHE_IDENTITY_LIMIT", "2048")

	configuration, loadError := Load()
	if loadError != nil {
		t.Fatal(loadError)
	}
	cache := configuration.Cache
	if cache.TokenTTL != 45*time.Second || cache.TokenLimit != 512 ||
		cache.ScopeTTL != time.Minute || cache.ScopeLimit != 128 ||
		cache.IdentityTTL != 2*time.Minute || cache.IdentityLimit != 2048 {
		t.Fatalf("cache environment overrides = %+v", cache)
	}
}

func TestLoadRejectsInvalidConnectionBounds(t *testing.T) {
	setTestEnvironment(t)
	t.Setenv("TRACEPILOT_DATABASE_MAX_CONNECTIONS", "2")
	t.Setenv("TRACEPILOT_DATABASE_MIN_CONNECTIONS", "3")
	if _, loadError := Load(); loadError == nil {
		t.Fatal("expected invalid connection bounds to fail")
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("TRACEPILOT_CLICKHOUSE_URL", "http://localhost:8123")
	if _, loadError := Load(); loadError == nil {
		t.Fatal("expected missing database URL to fail")
	}
}

func TestLoadRequiresClickHouseURL(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("TRACEPILOT_DATABASE_URL", "postgres://test")
	if _, loadError := Load(); loadError == nil {
		t.Fatal("expected missing ClickHouse URL to fail")
	}
}

func TestLoadReadsBackendEnvironmentFile(t *testing.T) {
	clearConfigEnvironment(t)
	repositoryDirectory := t.TempDir()
	backendDirectory := filepath.Join(repositoryDirectory, "backend")
	if makeDirectoryError := os.Mkdir(backendDirectory, 0o700); makeDirectoryError != nil {
		t.Fatal(makeDirectoryError)
	}
	if writeError := os.WriteFile(filepath.Join(backendDirectory, "go.mod"), []byte("module test\n"), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	contents := "TRACEPILOT_DATABASE_URL=postgres://repository\n" +
		"TRACEPILOT_DATABASE_AUTO_MIGRATE=true\n" +
		"TRACEPILOT_CLICKHOUSE_AUTO_MIGRATE=true\n" +
		"TRACEPILOT_CLICKHOUSE_URL=http://repository-ch\n" +
		"TRACEPILOT_HTTP_READ_TIMEOUT=9s\n" +
		"TRACEPILOT_HTTP_ALLOWED_ORIGINS=https://one.example, https://two.example\n"
	if writeError := os.WriteFile(filepath.Join(backendDirectory, ".env"), []byte(contents), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	t.Chdir(repositoryDirectory)

	configuration, loadError := Load()
	if loadError != nil {
		t.Fatal(loadError)
	}
	if configuration.Database.URL != "postgres://repository" {
		t.Fatalf("database URL = %q", configuration.Database.URL)
	}
	if !configuration.Database.AutoMigrate || !configuration.ClickHouse.AutoMigrate {
		t.Fatal("auto migration flag was not loaded from backend/.env")
	}
	if configuration.ClickHouse.URL != "http://repository-ch" {
		t.Fatalf("clickhouse URL = %q", configuration.ClickHouse.URL)
	}
	if configuration.HTTP.ReadTimeout != 9*time.Second || len(configuration.HTTP.AllowedOrigins) != 2 {
		t.Fatalf("typed .env values were not loaded: %+v", configuration.HTTP)
	}
}

func TestLoadAllowsDisabledRetention(t *testing.T) {
	setTestEnvironment(t)
	t.Setenv("TRACEPILOT_STORAGE_RETENTION_MAX_AGE", "0s")
	configuration, loadError := Load()
	if loadError != nil {
		t.Fatal(loadError)
	}
	if configuration.Storage.Retention.MaxAge != 0 {
		t.Fatalf("retention max age = %s", configuration.Storage.Retention.MaxAge)
	}
}

func TestRetentionDefaultsKeepTelemetryShorterThanHistory(t *testing.T) {
	setTestEnvironment(t)
	configuration, loadError := Load()
	if loadError != nil {
		t.Fatal(loadError)
	}
	day := 24 * time.Hour
	got := configuration.Storage.Retention
	if got.MaxAge != 7*day || got.IncidentMaxAge != 90*day || got.AuditMaxAge != 180*day || got.DeliveryMaxAge != 30*day {
		t.Fatalf("retention defaults = %+v", got)
	}
}

func TestRetentionRejectsNegativeAges(t *testing.T) {
	for _, name := range []string{"MAX_AGE", "INCIDENT_MAX_AGE", "AUDIT_MAX_AGE", "DELIVERY_MAX_AGE"} {
		setTestEnvironment(t)
		t.Setenv("TRACEPILOT_STORAGE_RETENTION_"+name, "-1h")
		if _, loadError := Load(); loadError == nil {
			t.Fatalf("negative %s was accepted", name)
		}
	}
}

func TestAPIQueryLimitsHaveSafeDefaultsAndRejectBadValues(t *testing.T) {
	setTestEnvironment(t)
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
		setTestEnvironment(t)
		t.Setenv(strings.TrimSpace(name), value)
		if _, loadError := Load(); loadError == nil {
			t.Fatalf("%s=%s was accepted", name, value)
		}
	}
}

func TestLoadAppliesDefaultsThenEnvironment(t *testing.T) {
	setTestEnvironment(t)
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

func TestLoadClickHouseLogPartitionFromEnvironment(t *testing.T) {
	for envVal, want := range map[string]LogPartition{
		"daily":   LogPartitionDaily,
		"monthly": LogPartitionMonthly,
		"yearly":  LogPartitionYearly,
		"weekly":  LogPartitionWeekly,
		"":        "",
	} {
		setTestEnvironment(t)
		if envVal != "" {
			t.Setenv("TRACEPILOT_CLICKHOUSE_LOG_PARTITION", envVal)
		}
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load with TRACEPILOT_CLICKHOUSE_LOG_PARTITION=%q failed: %v", envVal, err)
		}
		if cfg.ClickHouse.LogPartition != want {
			t.Fatalf("got LogPartition=%q, want %q", cfg.ClickHouse.LogPartition, want)
		}
	}
}
