package main

import "testing"

func TestMigrationRejectsInvalidArgumentsBeforeLoadingConfiguration(t *testing.T) {
	for _, args := range [][]string{
		{}, {"all", "extra"}, {"status", "extra"}, {"down"}, {"down", "0"},
		{"down", "-1"}, {"drop"}, {"postgres"}, {"postgres", "down"}, {"postgres", "status", "extra"},
		{"clickhouse"}, {"clickhouse", "status", "extra"},
		{"clickhouse", "all"}, {"clickhouse", "down", "1"}, {"clickhouse", "version"},
		{"postgres", "backfill"}, {"down", "1", "extra"}, {"--unknown"},
	} {
		if code := runMigrations(args); code != 2 {
			t.Errorf("migrate %v: exit=%d, want 2", args, code)
		}
	}
}

func TestPostgresMigrationParsesItsOwnCommands(t *testing.T) {
	for _, test := range []struct {
		arguments []string
		operation string
		steps     int
	}{
		{[]string{"up"}, "up", 0},
		{[]string{"down", "2"}, "down", 2},
		{[]string{"version"}, "version", 0},
		{[]string{"status"}, "status", 0},
	} {
		operation, steps, err := parsePostgresMigrationCommand(test.arguments)
		if err != nil || operation != test.operation || steps != test.steps {
			t.Errorf("postgres %v: operation=%s steps=%d error=%v", test.arguments, operation, steps, err)
		}
	}
	if _, _, err := parsePostgresMigrationCommand([]string{"backfill"}); err == nil {
		t.Fatal("PostgreSQL accepted a ClickHouse backfill command")
	}
}

func TestClickHouseMigrationParsesItsOwnCommands(t *testing.T) {
	for _, operation := range []string{"up", "status", "backfill"} {
		got, err := parseClickHouseMigrationCommand([]string{operation})
		if err != nil || got != operation {
			t.Errorf("clickhouse %s: operation=%s error=%v", operation, got, err)
		}
	}
	for _, arguments := range [][]string{{"down", "1"}, {"version"}, {"all"}} {
		if _, err := parseClickHouseMigrationCommand(arguments); err == nil {
			t.Errorf("ClickHouse accepted unsupported command %v", arguments)
		}
	}
}
