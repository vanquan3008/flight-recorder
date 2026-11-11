package config

import "testing"

func TestMigrationConfigurationRequiresOnlySelectedDatabase(t *testing.T) {
	for _, database := range []string{"postgres", "clickhouse"} {
		t.Run(database, func(t *testing.T) {
			clearConfigEnvironment(t)
			if database == "postgres" {
				t.Setenv("TRACEPILOT_DATABASE_URL", "postgres://migration-test")
			} else {
				t.Setenv("TRACEPILOT_CLICKHOUSE_URL", "http://migration-test:8123")
			}
			if _, err := LoadMigrations(database); err != nil {
				t.Fatalf("%s migration requires the other database: %v", database, err)
			}
			if _, err := LoadMigrations("all"); err == nil {
				t.Fatal("combined migration accepted a missing database URL")
			}
		})
	}
}

func TestMigrationIgnoresUnrelatedConfiguration(t *testing.T) {
	for _, database := range []string{"postgres", "clickhouse"} {
		t.Run(database, func(t *testing.T) {
			clearConfigEnvironment(t)
			t.Setenv("TRACEPILOT_AUTH_ACCESS_TOKEN_TTL", "invalid")
			t.Setenv("TRACEPILOT_HTTP_MAX_HEADER_BYTES", "invalid")
			if database == "postgres" {
				t.Setenv("TRACEPILOT_DATABASE_URL", "postgres://migration-test")
				t.Setenv("TRACEPILOT_CLICKHOUSE_URL", "invalid")
				t.Setenv("TRACEPILOT_CLICKHOUSE_REQUEST_TIMEOUT", "invalid")
				t.Setenv("TRACEPILOT_CLICKHOUSE_AUTO_MIGRATE", "invalid")
				t.Setenv("TRACEPILOT_RETENTION_MAX_AGE", "invalid")
			} else {
				t.Setenv("TRACEPILOT_CLICKHOUSE_URL", "http://migration-test:8123")
				t.Setenv("TRACEPILOT_DATABASE_URL", "invalid")
				t.Setenv("TRACEPILOT_DATABASE_MAX_CONNECTIONS", "invalid")
				t.Setenv("TRACEPILOT_DATABASE_AUTO_MIGRATE", "invalid")
			}
			if _, err := LoadMigrations(database); err != nil {
				t.Fatalf("%s migration parsed unrelated configuration: %v", database, err)
			}
			if _, err := LoadMigrations("all"); err == nil {
				t.Fatal("combined migration ignored invalid database configuration")
			}
		})
	}
}

func TestCombinedMigrationIgnoresApplicationConfiguration(t *testing.T) {
	setTestEnvironment(t)
	t.Setenv("TRACEPILOT_AUTH_ACCESS_TOKEN_TTL", "invalid")
	t.Setenv("TRACEPILOT_HTTP_MAX_HEADER_BYTES", "invalid")
	if _, err := LoadMigrations("all"); err != nil {
		t.Fatalf("combined migration required application configuration: %v", err)
	}
}

func TestMigrationStillRejectsInvalidSelectedConfiguration(t *testing.T) {
	for _, database := range []string{"postgres", "clickhouse"} {
		t.Run(database, func(t *testing.T) {
			setTestEnvironment(t)
			if database == "postgres" {
				t.Setenv("TRACEPILOT_DATABASE_CONNECT_TIMEOUT", "invalid")
			} else {
				t.Setenv("TRACEPILOT_CLICKHOUSE_REQUEST_TIMEOUT", "invalid")
			}
			if _, err := LoadMigrations(database); err == nil {
				t.Fatal("invalid selected database configuration was accepted")
			}
		})
	}
}

func TestMigrationRetainsProductionTLSChecks(t *testing.T) {
	for _, name := range []string{"TRACEPILOT_PRODUCTION", "TRACEPILOT_APP_PRODUCTION"} {
		t.Run(name, func(t *testing.T) {
			setTestEnvironment(t)
			t.Setenv(name, "true")
			for _, database := range []string{"postgres", "clickhouse"} {
				if _, err := LoadMigrations(database); err == nil {
					t.Errorf("%s migration bypassed production TLS checks with %s", database, name)
				}
			}
		})
	}
}

func TestAutoMigrationFlagsAreIndependent(t *testing.T) {
	setTestEnvironment(t)
	t.Setenv("TRACEPILOT_CLICKHOUSE_AUTO_MIGRATE", "true")
	cfg, err := Load()
	if err != nil || cfg.Database.AutoMigrate || !cfg.ClickHouse.AutoMigrate {
		t.Fatalf("ClickHouse flag changed PostgreSQL: pg=%t ch=%t error=%v", cfg.Database.AutoMigrate, cfg.ClickHouse.AutoMigrate, err)
	}
	t.Setenv("TRACEPILOT_DATABASE_AUTO_MIGRATE", "true")
	t.Setenv("TRACEPILOT_CLICKHOUSE_AUTO_MIGRATE", "false")
	cfg, err = Load()
	if err != nil || !cfg.Database.AutoMigrate || cfg.ClickHouse.AutoMigrate {
		t.Fatalf("PostgreSQL flag changed ClickHouse: pg=%t ch=%t error=%v", cfg.Database.AutoMigrate, cfg.ClickHouse.AutoMigrate, err)
	}
	t.Setenv("TRACEPILOT_CLICKHOUSE_AUTO_MIGRATE", "invalid")
	if _, err := Load(); err == nil {
		t.Fatal("invalid ClickHouse auto migration flag was accepted")
	}
}
