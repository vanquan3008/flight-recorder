package config

import (
	"net/http"
	"strings"
	"time"
)

// AuthConfig controls console token authentication, token lifecycles, and session cookie policies.
type AuthConfig struct {
	JWTSecret         string        `config:"jwt_secret"`
	IngestJWTSecret   string        `config:"ingest_jwt_secret"`
	AllowRegistration bool          `config:"allow_registration"`
	AccessTokenTTL    time.Duration `config:"access_token_ttl"`
	RefreshTokenTTL   time.Duration `config:"refresh_token_ttl"`

	CookieDomain   string `config:"cookie_domain"`
	CookieSameSite string `config:"cookie_same_site"`
	Issuer         string `config:"issuer"`
	Audience       string `config:"audience"`
}

// Sample secret markers are substrings that indicate a secret value is a sample or placeholder, not suitable for production use.
var sampleSecretMarkers = []string{
	"development",
	"dev-only",
	"example",
	"changeme",
	"change-me",
	"placeholder",
	"password123",
}

// EffectiveIngestJWTSecret returns IngestJWTSecret if configured, else falls back to JWTSecret.
func (auth AuthConfig) EffectiveIngestJWTSecret() string {
	if auth.IngestJWTSecret != "" {
		return auth.IngestJWTSecret
	}
	return auth.JWTSecret
}

// EffectiveIssuer returns the configured token issuer or "tracepilot".
func (auth AuthConfig) EffectiveIssuer() string {
	if auth.Issuer != "" {
		return auth.Issuer
	}
	return "tracepilot"
}

// EffectiveAudience returns the configured token audience or "tracepilot".
func (auth AuthConfig) EffectiveAudience() string {
	if auth.Audience != "" {
		return auth.Audience
	}
	return "tracepilot"
}

// CookieSameSiteMode maps the configured SameSite policy string to http.SameSite.
func (auth AuthConfig) CookieSameSiteMode() http.SameSite {
	switch strings.ToLower(strings.TrimSpace(auth.CookieSameSite)) {
	case "lax":
		return http.SameSiteLaxMode
	case "none":
		return http.SameSiteNoneMode
	case "strict", "":
		return http.SameSiteStrictMode
	default:
		return http.SameSiteStrictMode
	}
}

// Has sample secret markers checks if the provided value contains any known sample secret markers.
func hasSampleSecretMarker(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range sampleSecretMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func (configuration Config) validateAuth(validator *validator) {
	if configuration.Production && configuration.Auth.JWTSecret == "" {
		validator.add("auth.jwt_secret", "required in production")
	}
	if configuration.Production && configuration.Auth.IngestJWTSecret == "" {
		validator.add("auth.ingest_jwt_secret", "required in production")
	}
	if configuration.Auth.JWTSecret != "" && len(configuration.Auth.JWTSecret) < 32 {
		validator.add("auth.jwt_secret", "must contain at least 32 characters")
	}
	if configuration.Auth.IngestJWTSecret != "" && len(configuration.Auth.IngestJWTSecret) < 32 {
		validator.add("auth.ingest_jwt_secret", "must contain at least 32 characters")
	}

	// Token TTL validations
	if configuration.Auth.AccessTokenTTL <= 0 {
		validator.add("auth.access_token_ttl", "must be positive")
	} else if configuration.Auth.AccessTokenTTL > 24*time.Hour {
		validator.add("auth.access_token_ttl", "must not exceed 24 hours")
	}

	if configuration.Auth.RefreshTokenTTL <= 0 {
		validator.add("auth.refresh_token_ttl", "must be positive")
	} else if configuration.Auth.RefreshTokenTTL > 365*24*time.Hour {
		validator.add("auth.refresh_token_ttl", "must not exceed 365 days")
	}

	if configuration.Auth.AccessTokenTTL > 0 && configuration.Auth.RefreshTokenTTL > 0 &&
		configuration.Auth.RefreshTokenTTL < configuration.Auth.AccessTokenTTL {
		validator.add("auth.refresh_token_ttl", "must be greater than or equal to auth.access_token_ttl")
	}

	// Cookie validations
	if domain := configuration.Auth.CookieDomain; domain != "" {
		if strings.Contains(domain, "://") || strings.Contains(domain, "/") || strings.Contains(domain, ":") {
			validator.add("auth.cookie_domain", "must be a valid host or domain without scheme, path or port")
		}
	}

	if sameSite := strings.ToLower(strings.TrimSpace(configuration.Auth.CookieSameSite)); sameSite != "" {
		if sameSite != "strict" && sameSite != "lax" && sameSite != "none" {
			validator.add("auth.cookie_same_site", "must be one of 'strict', 'lax', or 'none'")
		}
	}

	if configuration.Production {
		validateProductionSecrets(configuration, validator)
	}
}

func validateProductionSecrets(configuration Config, validator *validator) {
	if hasSampleSecretMarker(configuration.Auth.JWTSecret) {
		validator.add("auth.jwt_secret", "sample value from repository; generate a random secret for production")
	}
	if configuration.Auth.IngestJWTSecret != "" && hasSampleSecretMarker(configuration.Auth.IngestJWTSecret) {
		validator.add("auth.ingest_jwt_secret", "sample value from repository; generate a random secret for production")
	}
	if configuration.Auth.JWTSecret != "" && configuration.Auth.IngestJWTSecret != "" &&
		configuration.Auth.JWTSecret == configuration.Auth.IngestJWTSecret {
		validator.add("auth.ingest_jwt_secret", "must not be identical to auth.jwt_secret in production to isolate telemetry ingestion from console access")
	}
	if hasSampleSecretMarker(configuration.HTTP.MetricsToken) {
		validator.add("http.metrics_token", "sample value from repository; generate a random token for production")
	}
	if hasSampleSecretMarker(configuration.Database.URL) {
		validator.add("database.url", "contains a sample credential from repository; use a real database password in production")
	}
}
