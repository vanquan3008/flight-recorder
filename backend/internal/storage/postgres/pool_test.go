package postgres

import (
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
)

func TestPoolConnectionTimeoutAndRenewal(t *testing.T) {
	for _, test := range []struct {
		name, query                      string
		timeout, wantTimeout, wantJitter time.Duration
	}{
		{name: "default", wantTimeout: 5 * time.Second, wantJitter: 6 * time.Minute},
		{name: "configured", timeout: 3 * time.Second, wantTimeout: 3 * time.Second, wantJitter: 6 * time.Minute},
		{name: "URL override", query: "?connect_timeout=7&pool_max_conn_lifetime_jitter=1m", timeout: 3 * time.Second, wantTimeout: 7 * time.Second, wantJitter: time.Minute},
	} {
		t.Run(test.name, func(t *testing.T) {
			pool, err := parsePoolConfig(config.DatabaseConfig{
				URL: "postgres://u:p@localhost/db" + test.query, MaxConnections: 20, MinConnections: 2,
				ConnectTimeout: test.timeout, MaxConnectionLifetime: time.Hour,
				MaxConnectionIdleTime: 30 * time.Minute, HealthCheckPeriod: time.Minute,
			})
			if err != nil {
				t.Fatal(err)
			}
			if pool.ConnConfig.ConnectTimeout != test.wantTimeout || pool.MaxConnLifetimeJitter != test.wantJitter {
				t.Fatalf("timeout=%v jitter=%v, want %v and %v", pool.ConnConfig.ConnectTimeout, pool.MaxConnLifetimeJitter, test.wantTimeout, test.wantJitter)
			}
		})
	}
}

func TestInvalidPoolSettingsDoNotExposeCredentials(t *testing.T) {
	_, err := parsePoolConfig(config.DatabaseConfig{URL: "postgres://u:private@localhost/db?connect_timeout=invalid"})
	if err == nil || strings.Contains(err.Error(), "private") {
		t.Fatalf("invalid settings exposed credentials or succeeded: %v", err)
	}
}
