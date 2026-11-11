package migrations

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/vanquan3008/tracepilot/db"
)

func CurrentVersion() (uint, error) { return db.LatestVersion(db.PostgresMigrations) }

// Run owns locking for mutations; callers use the same entry point for CLI and startup.
func Run(ctx context.Context, databaseURL, operation string, steps int) (string, error) {
	if operation != "up" && operation != "down" && operation != "version" && operation != "status" {
		return "", fmt.Errorf("unsupported migration operation %q", operation)
	}
	if operation == "down" && steps <= 0 {
		return "", errors.New("down requires a positive step count")
	}
	if operation == "version" || operation == "status" {
		state, err := Inspect(ctx, databaseURL)
		if err != nil {
			return "", err
		}
		if operation == "version" {
			if state.Version == 0 {
				return "version: none", nil
			}
			return fmt.Sprintf("version: %d, dirty: %t", state.Version, state.Dirty), nil
		}
		return state.String(), nil
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var result string
	err := withLock(ctx, databaseURL, func() error {
		var err error
		result, err = apply(ctx, databaseURL, operation, steps)
		return err
	})
	return result, err
}

func apply(ctx context.Context, databaseURL, operation string, steps int) (result string, runError error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	runner, err := open(ctx, databaseURL)
	if err != nil {
		return "", err
	}
	defer func() {
		sourceErr, databaseErr := runner.Close()
		runError = errors.Join(runError, sourceErr, databaseErr)
	}()
	stop := context.AfterFunc(ctx, func() { runner.GracefulStop <- true })
	defer stop()
	if operation == "up" {
		err = runner.Up()
		result = "migrations applied"
	} else {
		version, dirty, versionError := runner.Version()
		if versionError != nil {
			return "", versionError
		}
		if dirty {
			return "", fmt.Errorf("dirty database version %d; repair the schema before rollback", version)
		}
		if err := checkRollback(version, steps); err != nil {
			return "", err
		}
		err = runner.Steps(-steps)
		result = fmt.Sprintf("rolled back %d migration(s)", steps)
	}
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return "", err
	}
	return result, ctx.Err()
}

// Refuse missing down SQL before the runner can change the migration journal.
func checkRollback(version uint, steps int) error {
	history, err := db.List(db.PostgresMigrations)
	if err != nil {
		return err
	}
	for i := len(history) - 1; i >= 0; i-- {
		migration := history[i]
		if migration.Version > version {
			continue
		}
		path := strings.TrimSuffix(migration.File, ".up.sql") + ".down.sql"
		if _, err := db.SQL.ReadFile(path); err != nil {
			return fmt.Errorf("migration %d has no rollback SQL; use a new corrective migration", migration.Version)
		}
		steps--
		if steps == 0 {
			return nil
		}
	}
	return errors.New("rollback exceeds applied migration history")
}

func open(ctx context.Context, databaseURL string) (*migrate.Migrate, error) {
	source, err := iofs.New(db.SQL, db.PostgresMigrations)
	if err != nil {
		return nil, err
	}
	endpoint, err := connectionURL(databaseURL)
	if err != nil {
		_ = source.Close()
		return nil, err
	}
	endpoint.Scheme = "pgx5"
	query := endpoint.Query()
	timeout := 5 * time.Minute
	if deadline, ok := ctx.Deadline(); ok {
		timeout = min(timeout, time.Until(deadline))
	}
	// Bound the driver's background-context queries, including advisory-lock waits.
	query.Set("statement_timeout", strconv.FormatInt(max(1, timeout.Milliseconds()), 10))
	query.Set("x-statement-timeout", strconv.FormatInt(max(1, timeout.Milliseconds()), 10))
	endpoint.RawQuery = query.Encode()
	runner, err := migrate.NewWithSourceInstance("iofs", source, endpoint.String())
	if err != nil {
		_ = source.Close()
		return nil, fmt.Errorf("initialize migration runner: %w", err)
	}
	return runner, nil
}
