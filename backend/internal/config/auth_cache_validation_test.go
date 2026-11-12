package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidateCacheConfiguration(t *testing.T) {
	configuration := validConfig()

	if err := configuration.Validate(); err != nil {
		t.Fatalf("default cache configuration rejected: %v", err)
	}

	configuration.Cache.TokenTTL = -1
	if err := configuration.Validate(); err == nil {
		t.Fatal("expected negative token cache TTL to fail")
	}

	configuration = validConfig()
	configuration.Cache.IdentityLimit = -1
	if err := configuration.Validate(); err == nil {
		t.Fatal("expected negative identity cache limit to fail")
	}

	configuration = validConfig()
	configuration.Cache.DefaultTTL = -1
	if err := configuration.Validate(); err == nil {
		t.Fatal("expected negative default cache TTL to fail")
	}

	configuration = validConfig()
	configuration.Cache.Driver = "invalid-driver"
	if err := configuration.Validate(); err == nil {
		t.Fatal("expected invalid cache driver to fail")
	}

	configuration = validConfig()
	configuration.Cache.Driver = CacheDriverRedis
	configuration.Cache.URL = ""
	configuration.Cache.Address = ""
	if err := configuration.Validate(); err == nil {
		t.Fatal("expected redis driver without url or address to fail")
	}
}

func TestValidateAuthTokenLifecycles(t *testing.T) {
	valid := validConfig()

	if err := valid.Validate(); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}

	for name, change := range map[string]func(*Config){
		"zero access token TTL":            func(c *Config) { c.Auth.AccessTokenTTL = 0 },
		"negative access token TTL":        func(c *Config) { c.Auth.AccessTokenTTL = -time.Minute },
		"access token TTL exceeding 24h":   func(c *Config) { c.Auth.AccessTokenTTL = 25 * time.Hour },
		"zero refresh token TTL":           func(c *Config) { c.Auth.RefreshTokenTTL = 0 },
		"negative refresh token TTL":       func(c *Config) { c.Auth.RefreshTokenTTL = -time.Hour },
		"refresh token TTL exceeding 365d": func(c *Config) { c.Auth.RefreshTokenTTL = 366 * 24 * time.Hour },
		"refresh token TTL shorter than access token TTL": func(c *Config) {
			c.Auth.AccessTokenTTL = 2 * time.Hour
			c.Auth.RefreshTokenTTL = 1 * time.Hour
		},
	} {
		c := valid
		change(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestValidateAuthCookiePolicies(t *testing.T) {
	valid := validConfig()

	for name, change := range map[string]func(*Config){
		"invalid cookie domain with scheme": func(c *Config) { c.Auth.CookieDomain = "https://example.com" },
		"invalid cookie domain with path":   func(c *Config) { c.Auth.CookieDomain = "example.com/api" },
		"invalid cookie domain with port":   func(c *Config) { c.Auth.CookieDomain = "example.com:8080" },
		"invalid cookie same site":          func(c *Config) { c.Auth.CookieSameSite = "invalid-policy" },
	} {
		c := valid
		change(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	for _, policy := range []string{"strict", "lax", "none", "STRICT", "Lax"} {
		c := valid
		c.Auth.CookieSameSite = policy
		if err := c.Validate(); err != nil {
			t.Errorf("cookie policy %q was rejected: %v", policy, err)
		}
	}
}

func TestValidateProductionSecretsPrivilegeIsolation(t *testing.T) {
	production := func() Config {
		c := validConfig()
		c.Production = true
		c.Database.URL = "postgres://tracepilot:s3cure-random-pw@db.internal/tracepilot?sslmode=verify-full"
		c.Auth.JWTSecret = "b0f3a1c9e7d24a6f8c15e2d97a40b3c6f1e8d205"
		c.Auth.IngestJWTSecret = "c1a4e2d97a40b3c6f1e8d205b0f3a1c9e7d24a6f"
		c.TLS.CertificateFile = "certificate.pem"
		c.TLS.PrivateKeyFile = "private-key.pem"
		c.ClickHouse.URL = "https://ch.internal:8443"
		return c
	}

	if err := production().Validate(); err != nil {
		t.Fatalf("valid production configuration with separate secrets rejected: %v", err)
	}

	c := production()
	c.Auth.IngestJWTSecret = c.Auth.JWTSecret
	if err := c.Validate(); err == nil {
		t.Fatal("expected identical JWTSecret and IngestJWTSecret to be rejected in production")
	}

	c = production()
	c.Auth.IngestJWTSecret = ""
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "auth.ingest_jwt_secret: required in production") {
		t.Fatalf("expected a missing IngestJWTSecret to be rejected in production, got %v", err)
	}
}
