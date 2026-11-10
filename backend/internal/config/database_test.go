package config

import (
	"testing"
	"time"
)

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
