package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
)

const seedTimeout = 20 * time.Second

func seedAdminParams(cfg config.SeedConfig) (auth.CreateUserParams, error) {
	if cfg.AdminEmail == "" {
		return auth.CreateUserParams{}, errors.New("seed admin email is required; set TRACEPILOT_SEED_ADMIN_EMAIL")
	}
	if cfg.AdminPassword == "" {
		return auth.CreateUserParams{}, errors.New("seed admin password is required; set TRACEPILOT_SEED_ADMIN_PASSWORD")
	}
	name := cfg.AdminName
	if name == "" {
		name = "Admin"
	}
	return auth.CreateUserParams{
		Email:         cfg.AdminEmail,
		DisplayName:   name,
		Password:      cfg.AdminPassword,
		PlatformAdmin: true,
	}, nil
}

type userCreator func(context.Context, auth.CreateUserParams, auth.Actor) (auth.User, error)

func seedAdmin(ctx context.Context, cfg config.SeedConfig, create userCreator) (created bool, err error) {
	params, err := seedAdminParams(cfg)
	if err != nil {
		return false, err
	}
	if _, err := create(ctx, params, auth.Actor{}); err != nil {
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

	created, err := seedAdmin(ctx, cfg.Seed, database.CreateUser)
	if err != nil {
		logger.Error("seed execution failed", "error", err)
		return 1
	}
	if !created {
		logger.Info("development super admin account already exists", "email", cfg.Seed.AdminEmail)
		return 0
	}
	logger.Info("development super admin account seeded", "email", cfg.Seed.AdminEmail)
	return 0
}
