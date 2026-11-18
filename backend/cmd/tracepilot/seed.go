package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
)

const (
	seedTimeout   = 20 * time.Second
	adminEmail    = "admin@tracepilot.local"
	adminPassword = "admin-password-123"
)

func seedAdminParams() auth.CreateUserParams {
	return auth.CreateUserParams{Email: adminEmail, DisplayName: "Admin", Password: adminPassword, PlatformAdmin: true}
}

type userCreator func(context.Context, auth.CreateUserParams, auth.Actor) (auth.User, error)

func seedAdmin(ctx context.Context, create userCreator) (created bool, err error) {
	if _, err := create(ctx, seedAdminParams(), auth.Actor{}); err != nil {
		if errors.Is(err, auth.ErrEmailExists) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func runSeed(args []string) int {
	logger := slog.Default()
	if len(args) != 0 {
		logger.Error("invalid seed arguments", "usage", "tracepilot seed")
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		return 2
	}
	if cfg.Production {
		logger.Error("seed command is disabled in production")
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), seedTimeout)
	defer cancel()
	database, err := openOperatorDatabase(ctx, cfg.Database)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		return 1
	}
	defer database.Close()

	created, err := seedAdmin(ctx, database.CreateUser)
	if err != nil {
		logger.Error("seed execution failed", "error", err)
		return 1
	}
	if !created {
		logger.Info("development super admin account already exists", "email", adminEmail)
		return 0
	}
	logger.Info("development super admin account seeded", "email", adminEmail)
	return 0
}
