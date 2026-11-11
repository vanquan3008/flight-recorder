package config

import (
	"strconv"
	"testing"
	"time"
)

func TestDatabaseAutoMigrationDefaultsOffAndLoadsEnvironment(t *testing.T) {
	setTestEnvironment(t)
	cfg, err := Load()
	if err != nil || cfg.Database.AutoMigrate {
		t.Fatalf("auto migration must default off: enabled=%t error=%v", cfg.Database.AutoMigrate, err)
	}
	for _, enabled := range []bool{true, false} {
		t.Setenv("TRACEPILOT_DATABASE_AUTO_MIGRATE", strconv.FormatBool(enabled))
		cfg, err := Load()
		if err != nil || cfg.Database.AutoMigrate != enabled {
			t.Fatalf("auto migration enabled=%t, want=%t error=%v", cfg.Database.AutoMigrate, enabled, err)
		}
	}
	t.Setenv("TRACEPILOT_DATABASE_AUTO_MIGRATE", "invalid")
	if _, err := Load(); err == nil {
		t.Fatal("invalid auto migration flag was accepted")
	}
}

func TestDatabaseConnectionDefaultsAndEnvironment(t *testing.T) {
	setTestEnvironment(t)
	defaults := defaultConfig().Database
	if defaults.MaxConnections != 20 || defaults.ConnectTimeout != 5*time.Second {
		t.Fatalf("unexpected pool defaults: %+v", defaults)
	}
	t.Setenv("TRACEPILOT_DATABASE_CONNECT_TIMEOUT", "3s")
	cfg, err := Load()
	if err != nil || cfg.Database.ConnectTimeout != 3*time.Second {
		t.Fatalf("connect timeout override = %v, error = %v", cfg.Database.ConnectTimeout, err)
	}
}

func TestDatabaseRejectsUnboundedConnectTimeout(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second} {
		cfg := validConfig()
		cfg.Database.ConnectTimeout = timeout
		if err := cfg.Validate(); err == nil {
			t.Fatalf("accepted connect timeout %v", timeout)
		}
	}
}

func TestDatabaseCapacityAccountsForBothPipelines(t *testing.T) {
	cfg := validConfig()
	cfg.Ingestion.Workers = 2
	cfg.Database.MaxConnections = 5
	if err := cfg.Validate(); err == nil {
		t.Fatal("pool omitted metric workers from its connection budget")
	}
	cfg.Database.MaxConnections = 6
	if err := cfg.Validate(); err != nil {
		t.Fatalf("pool with room for both pipelines and two other operations: %v", err)
	}
}
