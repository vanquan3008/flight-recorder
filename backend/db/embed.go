// Package db embeds the schema history shipped with this application.
package db

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/golang-migrate/migrate/v4/source"
)

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
		return nil, err
	}
	var result []Migration
	seen := make(map[uint]bool)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		migration, err := source.DefaultParse(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("invalid migration filename %s: %w", entry.Name(), err)
		}
		if migration.Direction != source.Up {
			continue
		}
		if migration.Version == 0 || seen[migration.Version] {
			return nil, fmt.Errorf("invalid or duplicate migration version in %s", entry.Name())
		}
		seen[migration.Version] = true
		result = append(result, Migration{Version: migration.Version, File: directory + "/" + entry.Name()})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no up migrations in %s", directory)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Version < result[j].Version })
	return result, nil
}

func LatestVersion(directory string) (uint, error) {
	list, err := List(directory)
	if err != nil {
		return 0, err
	}
	return list[len(list)-1].Version, nil
}
