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
	var v validator
	base.validate(&v, true)
	if v.err() == nil {
		t.Fatal("production accepted plaintext ClickHouse")
	}
}
