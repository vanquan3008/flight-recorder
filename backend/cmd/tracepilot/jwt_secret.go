package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
)

func ensureJWTSecret(cfg *config.Config, logger *slog.Logger) error {
	log := logger.With("component", "authentication")

	if cfg.Auth.JWTSecret == "" {
		if cfg.Production {
			err := errors.New("TRACEPILOT_AUTH_JWT_SECRET is required in production")
			log.Error("JWT configuration invalid",
				"error", err,
				"environment", "production",
			)
			return err
		}

		secret, err := auth.NewJWTSecret()
		if err != nil {
			log.Error("failed to generate JWT secret", "error", err)
			return fmt.Errorf("generate JWT secret: %w", err)
		}

		cfg.Auth.JWTSecret = hex.EncodeToString(secret)

		log.Warn("ephemeral JWT secret generated",
			"environment", "development",
			"reason", "secret_not_configured",
			"warning", "existing tokens become invalid after restart",
		)
	} else {
		log.Info("JWT secret configured", "source", "configuration")
	}

	if cfg.Auth.IngestJWTSecret == "" {
		cfg.Auth.IngestJWTSecret = cfg.Auth.JWTSecret
		log.Warn("ingest JWT secret not configured",
			"fallback", "authentication_secret",
			"recommendation", "configure a separate ingest secret",
		)
	} else {
		log.Info("ingest JWT secret configured", "source", "configuration")
	}

	log.Info("JWT configuration initialized",
		"production", cfg.Production,
		"separate_ingest_secret", cfg.Auth.IngestJWTSecret != cfg.Auth.JWTSecret,
	)

	return nil
}
