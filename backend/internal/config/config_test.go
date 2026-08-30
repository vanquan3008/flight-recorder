package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var configEnvironment = []string{
	"TRACEPILOT_CONFIG_FILE",
	"TRACEPILOT_HTTP_ADDRESS",
	"TRACEPILOT_HTTP_ALLOWED_ORIGINS",
	"TRACEPILOT_HTTP_READ_HEADER_TIMEOUT",
	"TRACEPILOT_HTTP_READ_TIMEOUT",
	"TRACEPILOT_HTTP_WRITE_TIMEOUT",
	"TRACEPILOT_HTTP_IDLE_TIMEOUT",
	"TRACEPILOT_HTTP_MAX_HEADER_BYTES",
	"TRACEPILOT_HTTP_MANAGEMENT_MAX_BODY_BYTES",
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
	"TRACEPILOT_RETENTION_CHECK_INTERVAL",
	"TRACEPILOT_RETENTION_BATCH_SIZE",
	"TRACEPILOT_REPLAY_ENABLED",
	"TRACEPILOT_REPLAY_ALLOWED_HOSTS",
	"TRACEPILOT_REPLAY_ALLOW_PRIVATE",
	"TRACEPILOT_REPLAY_TIMEOUT",
	"TRACEPILOT_REPLAY_MAX_REQUEST_BYTES",
	"TRACEPILOT_REPLAY_MAX_RESPONSE_BYTES",
	"TRACEPILOT_REPLAY_MAX_CONCURRENT",
}

func TestLoadYAMLThenEnvironmentOverride(t *testing.T) {
	clearConfigEnvironment(t)
	configFile := filepath.Join(t.TempDir(), "tracepilot.yaml")
	contents := []byte("version: 1\nhttp:\n  address: ':9000'\ndatabase:\n  url: postgres://file\n  max_connections: 8\nlog_level: debug\n")
	if writeError := os.WriteFile(configFile, contents, 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	t.Setenv("TRACEPILOT_HTTP_ADDRESS", ":9100")
	t.Setenv("TRACEPILOT_DATABASE_URL", "postgres://environment")

	configuration, loadError := Load(configFile)
	if loadError != nil {
		t.Fatal(loadError)
	}
	if configuration.HTTP.Address != ":9100" || configuration.Database.URL != "postgres://environment" {
		t.Fatalf("environment did not override YAML: %+v", configuration)
	}
	if configuration.Database.MaxConnections != 8 || configuration.HTTP.ReadTimeout != 15*time.Second {
		t.Fatalf("YAML/default merge failed: %+v", configuration)
	}
}

func TestLoadRejectsUnknownYAMLField(t *testing.T) {
	clearConfigEnvironment(t)
	configFile := filepath.Join(t.TempDir(), "tracepilot.yaml")
	if writeError := os.WriteFile(configFile, []byte("version: 1\ndatabase:\n  url: postgres://test\nunknown: true\n"), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	if _, loadError := Load(configFile); loadError == nil {
		t.Fatal("expected unknown field to fail")
	}
}

func TestLoadRequiresYAMLVersion(t *testing.T) {
	clearConfigEnvironment(t)
	configFile := filepath.Join(t.TempDir(), "tracepilot.yaml")
	if writeError := os.WriteFile(configFile, []byte("database:\n  url: postgres://test\n"), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	if _, loadError := Load(configFile); loadError == nil {
		t.Fatal("expected missing configuration version to fail")
	}
}

func TestLoadRejectsInvalidConnectionBounds(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("TRACEPILOT_DATABASE_URL", "postgres://test")
	t.Setenv("TRACEPILOT_DATABASE_MAX_CONNECTIONS", "2")
	t.Setenv("TRACEPILOT_DATABASE_MIN_CONNECTIONS", "3")
	if _, loadError := Load(""); loadError == nil {
		t.Fatal("expected invalid connection bounds to fail")
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	clearConfigEnvironment(t)
	if _, loadError := Load(""); loadError == nil {
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
	if writeError := os.WriteFile(filepath.Join(repositoryDirectory, ".env"), []byte("TRACEPILOT_DATABASE_URL=postgres://repository\n"), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	t.Chdir(backendDirectory)

	configuration, loadError := Load("")
	if loadError != nil {
		t.Fatal(loadError)
	}
	if configuration.Database.URL != "postgres://repository" {
		t.Fatalf("database URL = %q", configuration.Database.URL)
	}
}

func TestValidateRejectsResourceExhaustingQueue(t *testing.T) {
	configuration := defaultConfig()
	configuration.Database.URL = "postgres://test"
	configuration.Ingestion.QueueSize = maxIngestionQueueSize + 1
	if validationError := configuration.Validate(); validationError == nil {
		t.Fatal("expected resource-exhausting queue size to fail")
	}
}

func TestValidateRejectsShortBootstrapAPIKey(t *testing.T) {
	configuration := defaultConfig()
	configuration.Database.URL = "postgres://test"
	configuration.Auth.BootstrapAPIKey = "too-short"
	if validationError := configuration.Validate(); validationError == nil {
		t.Fatal("expected short bootstrap key to fail")
	}
}

func TestValidateRequiresReplayAllowlist(t *testing.T) {
	configuration := defaultConfig()
	configuration.Database.URL = "postgres://test"
	configuration.Replay.Enabled = true
	if validationError := configuration.Validate(); validationError == nil {
		t.Fatal("expected enabled replay without allowlist to fail")
	}
}

func TestValidateRequiresCompleteTLSConfiguration(t *testing.T) {
	configuration := defaultConfig()
	configuration.Database.URL = "postgres://tracepilot:tracepilot@localhost/tracepilot"
	configuration.TLS.CertificateFile = "certificate.pem"
	if validateError := configuration.Validate(); validateError == nil {
		t.Fatal("expected incomplete TLS configuration to fail")
	}
}

func TestValidateProductionRequiresAuthAndTLS(t *testing.T) {
	configuration := defaultConfig()
	configuration.Database.URL = "postgres://tracepilot:tracepilot@localhost/tracepilot"
	configuration.Production = true
	if validateError := configuration.Validate(); validateError == nil {
		t.Fatal("expected production authentication and TLS to be required")
	}
	configuration.Auth.BootstrapAPIKey = "production-bootstrap-key-000000000000000000000000000000"
	configuration.TLS.CertificateFile = "certificate.pem"
	configuration.TLS.PrivateKeyFile = "private-key.pem"
	if validateError := configuration.Validate(); validateError != nil {
		t.Fatalf("production configuration rejected: %v", validateError)
	}
}

func TestValidateRejectsInvalidAllowedOrigin(t *testing.T) {
	configuration := defaultConfig()
	configuration.Database.URL = "postgres://test"
	configuration.HTTP.AllowedOrigins = []string{"https://operator.example.test/path"}
	if validateError := configuration.Validate(); validateError == nil {
		t.Fatal("expected invalid allowed origin to fail")
	}
}

func TestLoadAllowsDisabledRetention(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("TRACEPILOT_DATABASE_URL", "postgres://test")
	t.Setenv("TRACEPILOT_RETENTION_MAX_AGE", "0s")
	configuration, loadError := Load("")
	if loadError != nil {
		t.Fatal(loadError)
	}
	if configuration.Retention.MaxAge != 0 {
		t.Fatalf("retention max age = %s", configuration.Retention.MaxAge)
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
