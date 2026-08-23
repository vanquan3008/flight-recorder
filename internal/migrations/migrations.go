package migrations

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

const CurrentVersion uint = 3

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
	if filepath.VolumeName(absolutePath) != "" && !strings.HasPrefix(normalizedPath, "/") {
		normalizedPath = "/" + normalizedPath
	}
	sourceURL := (&url.URL{Scheme: "file", Path: normalizedPath}).String()

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
