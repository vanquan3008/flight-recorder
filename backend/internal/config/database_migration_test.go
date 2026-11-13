package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidateDatabaseMigrationAndPartitionSettings(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("default migration and partition settings rejected: %v", err)
	}
	for field, change := range map[string]func(*DatabaseConfig){
		"database.migration_lock_timeout":      func(d *DatabaseConfig) { d.MigrationLockTimeout = 0 },
		"database.migration_statement_timeout": func(d *DatabaseConfig) { d.MigrationStatementTimeout = 25 * time.Hour },
		"database.partition_window_days":       func(d *DatabaseConfig) { d.PartitionWindowDays = 2 },
		"database.partition_check_interval":    func(d *DatabaseConfig) { d.PartitionCheckInterval = time.Second },
	} {
		configuration := validConfig()
		change(&configuration.Database)
		if err := configuration.Validate(); err == nil || !strings.Contains(err.Error(), field) {
			t.Errorf("%s: invalid value accepted, err = %v", field, err)
		}
	}
}

func TestDatabaseMigrationSettingsComeFromEnvironment(t *testing.T) {
	setTestEnvironment(t)
	t.Setenv("TRACEPILOT_DATABASE_MIGRATION_LOCK_TIMEOUT", "30s")
	t.Setenv("TRACEPILOT_DATABASE_MIGRATION_STATEMENT_TIMEOUT", "15m")
	t.Setenv("TRACEPILOT_DATABASE_PARTITION_WINDOW_DAYS", "14")
	t.Setenv("TRACEPILOT_DATABASE_PARTITION_CHECK_INTERVAL", "30m")
	configuration, err := LoadMigrations("postgres")
	if err != nil {
		t.Fatal(err)
	}
	database := configuration.Database
	if database.MigrationLockTimeout != 30*time.Second || database.MigrationStatementTimeout != 15*time.Minute ||
		database.PartitionWindowDays != 14 || database.PartitionCheckInterval != 30*time.Minute {
		t.Fatalf("database settings = %+v", database)
	}
}
