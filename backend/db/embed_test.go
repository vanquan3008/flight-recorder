package db

import (
	"strings"
	"testing"
)

func TestEmbeddedHistoryIsOrderedAndComplete(t *testing.T) {
	for _, directory := range []string{PostgresMigrations, ClickHouseMigrations} {
		list, err := ListMigrations(directory)
		if err != nil {
			t.Fatal(err)
		}
		for i, migration := range list {
			if i > 0 && migration.Version <= list[i-1].Version {
				t.Fatalf("unordered history: %+v", list)
			}
			data, err := MigrationFiles.ReadFile(migration.FilePath)
			if err != nil || len(strings.TrimSpace(string(data))) == 0 {
				t.Fatalf("missing SQL for %s: %v", migration.FilePath, err)
			}
			if directory == PostgresMigrations && i > 0 {
				if _, err := MigrationFiles.ReadFile(strings.Replace(migration.FilePath, ".up.sql", ".down.sql", 1)); err != nil {
					t.Fatal(err)
				}
			}
		}
		latest, err := LatestVersion(directory)
		if err != nil || latest != list[len(list)-1].Version {
			t.Fatalf("latest does not match history: %d, %v", latest, err)
		}
	}
}
