package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
)

const createAdminUsage = "tracepilot create-admin --email ADDRESS --name NAME --password-stdin"

func parseCreateAdmin(arguments []string, input io.Reader) (auth.CreateUserParams, error) {
	var params auth.CreateUserParams
	var passwordStdin bool
	flags := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&params.Email, "email", "", "administrator email")
	flags.StringVar(&params.DisplayName, "name", "", "administrator display name")
	flags.BoolVar(&passwordStdin, "password-stdin", false, "read one password from stdin")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || !passwordStdin {
		return params, errors.New(createAdminUsage)
	}
	password, err := io.ReadAll(io.LimitReader(input, 4097))
	if err != nil || len(password) > 4096 {
		return params, errors.New("cannot read password: expected at most 4096 bytes")
	}
	params.Password = strings.TrimSuffix(strings.TrimSuffix(string(password), "\n"), "\r")
	if strings.ContainsAny(params.Password, "\r\n") {
		return params, errors.New("password must be a single line")
	}
	params.PlatformAdmin = true
	return auth.NormalizeCreateUserParams(params)
}

func runCreateAdmin(arguments []string, input io.Reader) int {
	params, err := parseCreateAdmin(arguments, input)
	if err != nil {
		slog.Error("invalid create-admin arguments", "error", err)
		return 2
	}
	// This operator command needs only PostgreSQL; retain its production TLS validation.
	cfg, err := config.LoadMigrations("postgres")
	if err != nil {
		slog.Error("invalid PostgreSQL configuration", "error", err)
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := openOperatorDatabase(ctx, cfg.Database)
	if err != nil {
		slog.Error("open PostgreSQL", "error", err)
		return 1
	}
	defer database.Close()
	user, err := database.CreateUser(ctx, params, auth.Actor{})
	if err != nil {
		slog.Error("create administrator failed", "error", err)
		return 1
	}
	slog.Info("administrator created", "id", user.ID, "email", user.Email)
	return 0
}

// openOperatorDatabase opens a small pool for one-off account commands and requires a migrated schema.
func openOperatorDatabase(ctx context.Context, cfg config.DatabaseConfig) (*postgres.Database, error) {
	cfg.MaxConnections, cfg.MinConnections = 2, 0
	database, err := postgres.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := database.Ready(ctx); err != nil {
		database.Close()
		return nil, fmt.Errorf("schema is not ready; run migrations first: %w", err)
	}
	return database, nil
}
