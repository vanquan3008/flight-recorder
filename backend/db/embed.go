package db

import (
	"embed"
	"fmt"
	"log/slog"
	"path"
	"sort"
	"strings"

	"github.com/golang-migrate/migrate/v4/source"
)

const (
	PostgresMigrations   = "postgres/migrations"
	ClickHouseMigrations = "clickhouse/migrations"
)

//go:embed postgres/migrations/*.sql clickhouse/migrations/*.sql
var MigrationFiles embed.FS

// Migration describes an embedded database migration.
type Migration struct {
	Version  uint
	FilePath string
}

// ListMigrations returns the ordered up migrations in directory.
func ListMigrations(directory string) ([]Migration, error) {
	entries, err := MigrationFiles.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf(
			"read migration directory %q: %w",
			directory,
			err,
		)
	}
	migrations := make([]Migration, 0, len(entries))
	registeredVersions := make(map[uint]string, len(entries))

	for _, entry := range entries {
		fileName := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(fileName, ".sql") {
			continue
		}
		parsedMigration, err := source.DefaultParse(fileName)
		if err != nil {
			return nil, fmt.Errorf(
				"parse migration file %q: %w",
				path.Join(directory, fileName),
				err,
			)
		}

		if parsedMigration.Direction != source.Up {
			continue
		}

		if err := validateMigrationVersion(
			parsedMigration.Version,
			fileName,
			directory,
			registeredVersions,
		); err != nil {
			return nil, err
		}

		registeredVersions[parsedMigration.Version] = fileName

		migrations = append(migrations, Migration{
			Version:  parsedMigration.Version,
			FilePath: path.Join(directory, fileName),
		})
	}

	if len(migrations) == 0 {
		return nil, fmt.Errorf(
			"no up migrations found in directory %q",
			directory,
		)
	}

	sort.Slice(migrations, func(firstIndex, secondIndex int) bool {
		return migrations[firstIndex].Version <
			migrations[secondIndex].Version
	})

	slog.Debug(
		"Database migrations loaded",
		slog.String("directory", directory),
		slog.Int("count", len(migrations)),
		slog.Uint64(
			"latest_version",
			uint64(migrations[len(migrations)-1].Version),
		),
	)

	return migrations, nil
}

// LatestVersion returns the highest embedded migration version.
func LatestVersion(directory string) (uint, error) {
	migrations, err := ListMigrations(directory)
	if err != nil {
		return 0, err
	}

	return migrations[len(migrations)-1].Version, nil
}

// validateMigrationVersion checks if the migration version is valid and not duplicated.
func validateMigrationVersion(
	version uint,
	fileName string,
	directory string,
	registeredVersions map[uint]string,
) error {
	if version == 0 {
		return fmt.Errorf(
			"invalid migration version 0 in file %q",
			path.Join(directory, fileName),
		)
	}

	existingFileName, exists := registeredVersions[version]
	if exists {
		return fmt.Errorf(
			"duplicate migration version %d in directory %q: files %q and %q",
			version,
			directory,
			existingFileName,
			fileName,
		)
	}

	return nil
}
