package main

import (
	"bytes"
	"encoding/json"
	"log"
	"log/slog"
	"strings"
	"testing"
)

func TestMigrationRejectsInvalidArgumentsBeforeLoadingConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("TRACEPILOT_PRODUCTION", "invalid")
	var output bytes.Buffer
	logger, writer, flags := slog.Default(), log.Writer(), log.Flags()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() {
		slog.SetDefault(logger)
		log.SetOutput(writer)
		log.SetFlags(flags)
	})
	for _, test := range []struct {
		command  string
		database string
	}{
		{"", ""}, {"all extra", ""}, {"status extra", ""}, {"down", ""},
		{"down 0", ""}, {"down -1", ""}, {"drop", ""}, {"postgres", ""},
		{"postgres down", ""}, {"postgres status extra", ""}, {"postgres all extra", ""},
		{"clickhouse", ""}, {"clickhouse status extra", ""}, {"clickhouse all", ""},
		{"clickhouse down 1", ""}, {"clickhouse version", ""}, {"postgres backfill", ""},
		{"down 1 extra", ""}, {"--unknown", ""},
		{"status", "postgres"}, {"postgres down 2", "postgres"}, {"clickhouse status", "clickhouse"},
		{"all", "all"}, {"postgres all", "all"},
	} {
		output.Reset()
		if code := runMigrations(strings.Fields(test.command)); code != 2 {
			t.Errorf("migrate %q: exit=%d, want 2", test.command, code)
		}
		var entry struct {
			Message  string `json:"msg"`
			Database string `json:"database"`
		}
		if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
			t.Fatalf("migrate %q: decode log: %v", test.command, err)
		}
		wantMessage := "invalid migration arguments"
		if test.database != "" {
			wantMessage = "invalid configuration"
		}
		if entry.Message != wantMessage || entry.Database != test.database {
			t.Errorf("migrate %q: message=%q database=%q, want %q database=%q",
				test.command, entry.Message, entry.Database, wantMessage, test.database)
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
	for _, operation := range []string{"up", "status"} {
		got, err := parseClickHouseMigrationCommand([]string{operation})
		if err != nil || got != operation {
			t.Errorf("clickhouse %s: operation=%s error=%v", operation, got, err)
		}
	}
	// backfill copied legacy PostgreSQL logs; logs now live only in ClickHouse.
	for _, arguments := range [][]string{{"down", "1"}, {"version"}, {"all"}, {"backfill"}} {
		if _, err := parseClickHouseMigrationCommand(arguments); err == nil {
			t.Errorf("ClickHouse accepted unsupported command %v", arguments)
		}
	}
}
