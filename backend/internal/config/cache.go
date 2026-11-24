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
)

type CacheConfig struct {
	Driver        string        `config:"driver"`
	URL           string        `config:"url"`
	Address       string        `config:"address"`
	Password      string        `config:"password"`
	DB            int           `config:"db"`
	DefaultTTL    time.Duration `config:"default_ttl"`
	DefaultLimit  int           `config:"default_limit"`
	TokenTTL      time.Duration `config:"token_ttl"`
	TokenLimit    int           `config:"token_limit"`
	ScopeTTL      time.Duration `config:"scope_ttl"`
	ScopeLimit    int           `config:"scope_limit"`
	IdentityTTL   time.Duration `config:"identity_ttl"`
	IdentityLimit int           `config:"identity_limit"`
}

// EffectiveDriver returns the resolved cache driver.
func (c CacheConfig) EffectiveDriver() string {
	d := strings.ToLower(strings.TrimSpace(c.Driver))
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

func (configuration Config) validateCache(validator *validator) {
	configuration.Cache.validate(validator)
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
