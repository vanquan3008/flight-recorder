package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var configEnvironment = []string{
	"FLIGHT_CONFIG_FILE",
	"FLIGHT_HTTP_ADDRESS",
	"FLIGHT_HTTP_READ_HEADER_TIMEOUT",
	"FLIGHT_HTTP_READ_TIMEOUT",
	"FLIGHT_HTTP_WRITE_TIMEOUT",
	"FLIGHT_HTTP_IDLE_TIMEOUT",
	"FLIGHT_HTTP_MAX_HEADER_BYTES",
	"FLIGHT_HTTP_MANAGEMENT_MAX_BODY_BYTES",
	"FLIGHT_INGESTION_GRPC_ADDRESS",
	"FLIGHT_INGESTION_HTTP_ADDRESS",
	"FLIGHT_INGESTION_PROJECT_ID",
	"FLIGHT_INGESTION_PROJECT_SLUG",
	"FLIGHT_INGESTION_PROJECT_NAME",
	"FLIGHT_INGESTION_ENVIRONMENT_ID",
	"FLIGHT_INGESTION_ENVIRONMENT_NAME",
	"FLIGHT_INGESTION_MAX_REQUEST_BYTES",
	"FLIGHT_INGESTION_MAX_RECORDS_PER_REQUEST",
	"FLIGHT_INGESTION_MAX_ATTRIBUTES_PER_RECORD",
	"FLIGHT_INGESTION_MAX_ATTRIBUTE_VALUE_BYTES",
	"FLIGHT_INGESTION_MAX_ATTRIBUTE_DEPTH",
	"FLIGHT_INGESTION_QUEUE_SIZE",
	"FLIGHT_INGESTION_QUEUE_MAX_BYTES",
	"FLIGHT_INGESTION_ENQUEUE_TIMEOUT",
	"FLIGHT_INGESTION_BATCH_SIZE",
	"FLIGHT_INGESTION_BATCH_TIMEOUT",
	"FLIGHT_INGESTION_FLUSH_INTERVAL",
	"FLIGHT_INGESTION_WORKERS",
	"FLIGHT_INGESTION_REDACTED_KEYS",
	"FLIGHT_SHUTDOWN_TIMEOUT",
	"FLIGHT_LOG_LEVEL",
	"FLIGHT_DATABASE_URL",
	"FLIGHT_DATABASE_MAX_CONNECTIONS",
	"FLIGHT_DATABASE_MIN_CONNECTIONS",
	"FLIGHT_DATABASE_MAX_CONNECTION_LIFETIME",
	"FLIGHT_DATABASE_MAX_CONNECTION_IDLE_TIME",
	"FLIGHT_DATABASE_HEALTH_CHECK_PERIOD",
	"FLIGHT_DATABASE_READINESS_TIMEOUT",
	"FLIGHT_MIGRATIONS_PATH",
}

func TestLoadYAMLThenEnvironmentOverride(t *testing.T) {
	clearConfigEnvironment(t)
	configFile := filepath.Join(t.TempDir(), "flight.yaml")
	contents := []byte("version: 1\nhttp:\n  address: ':9000'\ndatabase:\n  url: postgres://file\n  max_connections: 8\nlog_level: debug\n")
	if writeError := os.WriteFile(configFile, contents, 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	t.Setenv("FLIGHT_HTTP_ADDRESS", ":9100")
	t.Setenv("FLIGHT_DATABASE_URL", "postgres://environment")

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
	configFile := filepath.Join(t.TempDir(), "flight.yaml")
	if writeError := os.WriteFile(configFile, []byte("version: 1\ndatabase:\n  url: postgres://test\nunknown: true\n"), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	if _, loadError := Load(configFile); loadError == nil {
		t.Fatal("expected unknown field to fail")
	}
}

func TestLoadRequiresYAMLVersion(t *testing.T) {
	clearConfigEnvironment(t)
	configFile := filepath.Join(t.TempDir(), "flight.yaml")
	if writeError := os.WriteFile(configFile, []byte("database:\n  url: postgres://test\n"), 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	if _, loadError := Load(configFile); loadError == nil {
		t.Fatal("expected missing configuration version to fail")
	}
}

func TestLoadRejectsInvalidConnectionBounds(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("FLIGHT_DATABASE_URL", "postgres://test")
	t.Setenv("FLIGHT_DATABASE_MAX_CONNECTIONS", "2")
	t.Setenv("FLIGHT_DATABASE_MIN_CONNECTIONS", "3")
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

func TestValidateRejectsResourceExhaustingQueue(t *testing.T) {
	configuration := defaultConfig()
	configuration.Database.URL = "postgres://test"
	configuration.Ingestion.QueueSize = maxIngestionQueueSize + 1
	if validationError := configuration.Validate(); validationError == nil {
		t.Fatal("expected resource-exhausting queue size to fail")
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
