// Package db embeds the schema history shipped with this application.
package db

import (
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/golang-migrate/migrate/v4/source"
)

// Directories of the two independent migration histories inside SQL.
const (
	PostgresMigrations   = "postgres/migrations"
	ClickHouseMigrations = "clickhouse/migrations"
)

//go:embed postgres/migrations/*.sql clickhouse/migrations/*.sql
var SQL embed.FS

type Migration struct {
	Version uint
	File    string
}

func List(directory string) ([]Migration, error) {
	entries, err := SQL.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read migrations from %q: %w", directory, err)
	}

	migrations := make([]Migration, 0, len(entries))
	versions := make(map[uint]struct{}, len(entries))

	for _, entry := range entries {
		filename := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(filename, ".sql") {
			continue
		}

		parsed, err := source.DefaultParse(filename)
		if err != nil {
			return nil, fmt.Errorf("parse migration %q: %w", filename, err)
		}

		if parsed.Direction != source.Up {
			continue
		}

		if parsed.Version == 0 {
			return nil, fmt.Errorf("invalid migration version in %q", filename)
		}

		if _, exists := versions[parsed.Version]; exists {
			return nil, fmt.Errorf(
				"duplicate migration version %d in %q",
				parsed.Version,
				directory,
			)
		}

		versions[parsed.Version] = struct{}{}
		migrations = append(migrations, Migration{
			Version: parsed.Version,
			File:    directory + "/" + filename,
		})
	}

	if len(migrations) == 0 {
		return nil, fmt.Errorf("no up migrations found in %q", directory)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	slog.Debug("Database migrations loaded",
		"directory", directory,
		"count", len(migrations),
		"latest_version", migrations[len(migrations)-1].Version,
	)

	return migrations, nil
}

func LatestVersion(directory string) (uint, error) {
	migrations, err := List(directory)
	if err != nil {
		return 0, err
	}

	version := migrations[len(migrations)-1].Version

	slog.Debug("Latest migration version resolved",
		"directory", directory,
		"version", version,
	)

	return version, nil
}
