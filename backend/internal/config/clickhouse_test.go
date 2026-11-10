package config

import (
	"strings"
	"testing"
	"time"
)

func TestClickHouseValidation(t *testing.T) {
	base := defaultConfig().ClickHouse
	base.URL = "https://logs.example.com:8443"
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*ClickHouseConfig){
		func(c *ClickHouseConfig) { c.URL = "http://user:private@example.com" },
		func(c *ClickHouseConfig) { c.URL += "/?query=private" },
		func(c *ClickHouseConfig) { c.URL = "file:///private" },
		func(c *ClickHouseConfig) { c.Database = "db; DROP DATABASE default" },
		func(c *ClickHouseConfig) { c.Password = "private\nheader" },
		func(c *ClickHouseConfig) { c.RequestTimeout = 2 * time.Minute },
		func(c *ClickHouseConfig) { c.MaxConnections = 0 },
	} {
		cfg := base
		change(&cfg)
		if err := cfg.Validate(); err == nil || strings.Contains(err.Error(), "private") {
			t.Fatalf("invalid or sensitive validation error: %v", err)
		}
	}
	base.URL = "http://logs.example.com:8123"
	var validator validator
	base.validate(&validator, true)
	if validator.err() == nil {
		t.Fatal("production accepted plaintext ClickHouse")
	}
}

// TestClickHouseURLRequired verifies that an empty URL is now rejected.
func TestClickHouseURLRequired(t *testing.T) {
	cfg := defaultConfig().ClickHouse
	cfg.URL = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected empty ClickHouse URL to be rejected")
	}
}

// TestClickHouseLogPartition verifies that only the four allowed granularities
// and the empty string (which maps to the weekly default) are accepted.
func TestClickHouseLogPartition(t *testing.T) {
	base := defaultConfig().ClickHouse
	base.URL = "https://logs.example.com:8443"

	for _, partition := range []LogPartition{
		"", LogPartitionWeekly, LogPartitionDaily, LogPartitionMonthly, LogPartitionYearly,
	} {
		cfg := base
		cfg.LogPartition = partition
		if err := cfg.Validate(); err != nil {
			t.Errorf("partition %q should be valid, got: %v", partition, err)
		}
	}

	cfg := base
	cfg.LogPartition = "hourly"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected unknown partition granularity to be rejected")
	}
}

// TestLogPartitionExpr checks that every granularity produces a non-empty,
// distinct ClickHouse PARTITION BY expression.
func TestLogPartitionExpr(t *testing.T) {
	seen := map[string]LogPartition{}
	for _, p := range []LogPartition{LogPartitionDaily, LogPartitionMonthly, LogPartitionYearly, LogPartitionWeekly} {
		expr := p.PartitionExpr()
		if expr == "" {
			t.Errorf("partition %q returned empty expression", p)
		}
		if prev, ok := seen[expr]; ok {
			t.Errorf("partition %q and %q share the same expression %q", p, prev, expr)
		}
		seen[expr] = p
	}
	if LogPartition("").PartitionExpr() != LogPartitionWeekly.PartitionExpr() {
		t.Errorf("empty partition must default to weekly")
	}
}

