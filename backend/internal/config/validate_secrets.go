package config

import (
	"errors"
	"strings"
)

// sampleSecretMarkers appear in the secrets shipped in .env.example and the
// README. A production deployment that kept one of them is reachable with a
// value anyone can read in the repository, so startup refuses it.
var sampleSecretMarkers = []string{"development", "dev-only", "example", "changeme", "change-me", "placeholder", "password123"}

func hasSampleSecretMarker(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range sampleSecretMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func validateProductionSecrets(configuration Config) error {
	if hasSampleSecretMarker(configuration.Auth.JWTSecret) {
		return errors.New("auth.jwt_secret is a sample value from the repository; generate a random secret for production")
	}
	if hasSampleSecretMarker(configuration.HTTP.MetricsToken) {
		return errors.New("http.metrics_token is a sample value from the repository; generate a random token for production")
	}
	if hasSampleSecretMarker(configuration.Database.URL) {
		return errors.New("database.url contains a sample credential from the repository; use a real database password in production")
	}
	return nil
}
