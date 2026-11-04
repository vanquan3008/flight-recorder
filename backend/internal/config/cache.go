package config

import (
	"strings"
	"time"
)

const (
	CacheDriverAuto   = "auto"
	CacheDriverRedis  = "redis"
	CacheDriverMemory = "memory"
	CacheDriverNoop   = "noop"

	// Backward compatibility constants
	CacheTypeMemory = CacheDriverMemory
	CacheTypeRedis  = CacheDriverRedis
	CacheTypeNoop   = CacheDriverNoop
)

// CacheConfig controls distributed (Redis) and in-memory caching policies.
type CacheConfig struct {
	// Driver selects the cache backend: "auto", "redis", "memory", or "noop".
	// In "auto" mode, Redis is used when a URL or Address is configured, with automatic fallback to in-memory caching.
	Driver string `config:"driver"`

	// Redis connection settings
	URL      string `config:"url"`      // e.g. "redis://[:password@]host:port/db"
	Address  string `config:"address"`  // e.g. "127.0.0.1:6379"
	Password string `config:"password"` // optional Redis authentication password
	DB       int    `config:"db"`       // optional Redis logical database index

	// General cache lifecycle policies
	DefaultTTL   time.Duration `config:"default_ttl"`   // Default expiration for cached items (default: 60s)
	DefaultLimit int           `config:"default_limit"` // Maximum item capacity for in-memory store (default: 4096)

	// In-memory entity overrides (optional, fallback to DefaultTTL and DefaultLimit if unset)
	TokenTTL      time.Duration `config:"token_ttl"`
	TokenLimit    int           `config:"token_limit"`
	ScopeTTL      time.Duration `config:"scope_ttl"`
	ScopeLimit    int           `config:"scope_limit"`
	IdentityTTL   time.Duration `config:"identity_ttl"`
	IdentityLimit int           `config:"identity_limit"`

	// Compatibility aliases with previous config
	Enabled bool          `config:"enabled"`
	Type    string        `config:"type"`
	TTL     time.Duration `config:"ttl"`
}

// EffectiveDriver returns the resolved cache driver.
func (c CacheConfig) EffectiveDriver() string {
	d := strings.ToLower(strings.TrimSpace(c.Driver))
	if d == "" {
		d = strings.ToLower(strings.TrimSpace(c.Type))
	}
	if d == "" || d == CacheDriverAuto {
		if strings.TrimSpace(c.URL) != "" || strings.TrimSpace(c.Address) != "" {
			return CacheDriverRedis
		}
		return CacheDriverMemory
	}
	return d
}

// EffectiveTokenTTL returns the TTL for token caching, falling back to DefaultTTL.
func (c CacheConfig) EffectiveTokenTTL() time.Duration {
	if c.TokenTTL > 0 {
		return c.TokenTTL
	}
	if c.DefaultTTL > 0 {
		return c.DefaultTTL
	}
	return 30 * time.Second
}

// EffectiveTokenLimit returns the capacity limit for token caching, falling back to DefaultLimit.
func (c CacheConfig) EffectiveTokenLimit() int {
	if c.TokenLimit > 0 {
		return c.TokenLimit
	}
	if c.DefaultLimit > 0 {
		return c.DefaultLimit
	}
	return 4096
}

// EffectiveScopeTTL returns the TTL for scope caching, falling back to DefaultTTL.
func (c CacheConfig) EffectiveScopeTTL() time.Duration {
	if c.ScopeTTL > 0 {
		return c.ScopeTTL
	}
	if c.DefaultTTL > 0 {
		return c.DefaultTTL
	}
	return 30 * time.Second
}

// EffectiveScopeLimit returns the capacity limit for scope caching, falling back to DefaultLimit.
func (c CacheConfig) EffectiveScopeLimit() int {
	if c.ScopeLimit > 0 {
		return c.ScopeLimit
	}
	if c.DefaultLimit > 0 {
		return c.DefaultLimit
	}
	return 1024
}

// EffectiveIdentityTTL returns the TTL for identity caching, falling back to DefaultTTL.
func (c CacheConfig) EffectiveIdentityTTL() time.Duration {
	if c.IdentityTTL > 0 {
		return c.IdentityTTL
	}
	if c.DefaultTTL > 0 {
		return c.DefaultTTL
	}
	return 30 * time.Second
}

// EffectiveIdentityLimit returns the capacity limit for identity caching, falling back to DefaultLimit.
func (c CacheConfig) EffectiveIdentityLimit() int {
	if c.IdentityLimit > 0 {
		return c.IdentityLimit
	}
	if c.DefaultLimit > 0 {
		return c.DefaultLimit
	}
	return 4096
}

func (configuration Config) validateCache(v *validator) {
	configuration.Cache.validate(v)
}

func (c CacheConfig) validate(validator *validator) {
	if c.DefaultTTL < 0 {
		validator.add("cache.default_ttl", "must not be negative")
	}
	if c.DefaultLimit < 0 {
		validator.add("cache.default_limit", "must not be negative")
	}
	if c.TokenTTL < 0 {
		validator.add("cache.token_ttl", "must not be negative")
	}
	if c.TokenLimit < 0 {
		validator.add("cache.token_limit", "must not be negative")
	}
	if c.ScopeTTL < 0 {
		validator.add("cache.scope_ttl", "must not be negative")
	}
	if c.ScopeLimit < 0 {
		validator.add("cache.scope_limit", "must not be negative")
	}
	if c.IdentityTTL < 0 {
		validator.add("cache.identity_ttl", "must not be negative")
	}
	if c.IdentityLimit < 0 {
		validator.add("cache.identity_limit", "must not be negative")
	}

	driver := c.EffectiveDriver()
	switch driver {
	case CacheDriverRedis, CacheDriverMemory, CacheDriverNoop:
	default:
		validator.add("cache.driver", "must be one of: auto, redis, memory, noop")
		return
	}

	if driver == CacheDriverRedis && strings.TrimSpace(c.URL) == "" && strings.TrimSpace(c.Address) == "" {
		validator.add("cache.address", "must specify cache.url or cache.address when driver is redis")
	}

	if c.DB < 0 {
		validator.add("cache.db", "must not be negative")
	}
}
