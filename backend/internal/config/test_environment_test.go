package config

import (
	"os"
	"reflect"
	"testing"
)

func configEnvironment() []string {
	var names []string
	var walk func(reflect.Type, []string)
	walk = func(structType reflect.Type, path []string) {
		for index := range structType.NumField() {
			field := structType.Field(index)
			fieldPath := appendPath(path, configName(field))
			if field.Type.Kind() == reflect.Struct && !reflect.PointerTo(field.Type).Implements(textUnmarshalerType) {
				walk(field.Type, fieldPath)
				continue
			}
			names = append(names, environmentName(fieldPath))
		}
	}
	walk(reflect.TypeFor[Config](), nil)
	return names
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	names := configEnvironment()
	for _, name := range names {
		if legacy := legacyEnvironmentName(name); legacy != "" {
			names = append(names, legacy)
		}
	}
	for _, name := range names {
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

// validConfig returns a default configuration that passes Validate() with
// all required fields populated (PostgreSQL and ClickHouse URLs).
func validConfig() Config {
	cfg := defaultConfig()
	cfg.Database.URL = "postgres://test"
	cfg.ClickHouse.URL = "https://ch.example.com:8443"
	return cfg
}

// setTestEnvironment clears the environment and sets minimal valid variables
// so Load() can succeed without a .env file.
func setTestEnvironment(t *testing.T) {
	t.Helper()
	clearConfigEnvironment(t)
	t.Setenv("TRACEPILOT_DATABASE_URL", "postgres://test")
	t.Setenv("TRACEPILOT_CLICKHOUSE_URL", "http://localhost:8123")
}
