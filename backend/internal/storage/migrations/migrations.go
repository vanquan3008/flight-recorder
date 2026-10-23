package migrations

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// CurrentVersion is the schema version the binary expects; bump it with every new migration.
const CurrentVersion uint = 1

func EnsureCurrent(databaseURL, migrationsPath string) (runError error) {
	migrationRunner, openError := open(databaseURL, migrationsPath)
	if openError != nil {
		return openError
	}
	defer func() {
		sourceCloseError, databaseCloseError := migrationRunner.Close()
		runError = errors.Join(runError, sourceCloseError, databaseCloseError)
	}()
	version, dirty, versionError := migrationRunner.Version()
	return validateCurrentVersion(version, dirty, versionError)
}

func validateCurrentVersion(version uint, dirty bool, versionError error) error {
	if errors.Is(versionError, migrate.ErrNilVersion) {
		return errors.New("database schema is not initialized; run tracepilot migrate up")
	}
	if versionError != nil {
		return fmt.Errorf("read database schema version: %w", versionError)
	}
	if dirty {
		return fmt.Errorf("database schema version %d is dirty; repair it before starting TracePilot", version)
	}
	if version != CurrentVersion {
		return fmt.Errorf("database schema version %d does not match binary version %d; run tracepilot migrate up", version, CurrentVersion)
	}
	return nil
}

func Run(databaseURL, migrationsPath, operation string, steps int) (result string, runError error) {
	if operation != "up" && operation != "down" && operation != "version" {
		return "", fmt.Errorf("unsupported migration operation %q", operation)
	}
	if operation == "down" && steps <= 0 {
		return "", errors.New("down requires a positive step count")
	}

	migrationRunner, openError := open(databaseURL, migrationsPath)
	if openError != nil {
		return "", openError
	}
	defer func() {
		sourceCloseError, databaseCloseError := migrationRunner.Close()
		runError = errors.Join(runError, sourceCloseError, databaseCloseError)
	}()

	switch operation {
	case "up":
		if migrationError := migrationRunner.Up(); migrationError != nil && !errors.Is(migrationError, migrate.ErrNoChange) {
			return "", migrationError
		}
		return "migrations applied", nil
	case "down":
		if migrationError := migrationRunner.Steps(-steps); migrationError != nil {
			return "", migrationError
		}
		return fmt.Sprintf("rolled back %d migration(s)", steps), nil
	case "version":
		version, dirty, versionError := migrationRunner.Version()
		if errors.Is(versionError, migrate.ErrNilVersion) {
			return "version: none", nil
		}
		if versionError != nil {
			return "", versionError
		}
		return fmt.Sprintf("version: %d, dirty: %t", version, dirty), nil
	}
	return "", errors.New("unreachable migration operation")
}

func open(databaseURL, migrationsPath string) (*migrate.Migrate, error) {
	absolutePath, pathError := filepath.Abs(migrationsPath)
	if pathError != nil {
		return nil, errors.New("resolve migrations path")
	}
	normalizedPath := filepath.ToSlash(absolutePath)
	// Use URL opaque form so Windows drive letters are not interpreted as a
	// POSIX `/D:/...` path by golang-migrate's file source driver.
	sourceURL := (&url.URL{Scheme: "file", Opaque: normalizedPath}).String()

	parsedDatabaseURL, parseError := url.Parse(databaseURL)
	if parseError != nil {
		return nil, errors.New("parse migration database URL")
	}
	switch parsedDatabaseURL.Scheme {
	case "postgres", "postgresql", "pgx5":
		parsedDatabaseURL.Scheme = "pgx5"
	default:
		return nil, errors.New("migration database URL must use postgres or postgresql scheme")
	}

	migrationRunner, migrationError := migrate.New(sourceURL, parsedDatabaseURL.String())
	if migrationError != nil {
		return nil, fmt.Errorf("initialize migration runner: %w", migrationError)
	}
	return migrationRunner, nil
}
