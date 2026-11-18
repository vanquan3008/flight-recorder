package clickhouse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestMigrationStatusIsReadOnlyForMissingDatabase(t *testing.T) {
	store := testStore(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if !strings.HasPrefix(query.Get("query"), "SELECT ") || query.Get("database") != "default" {
			t.Error("fresh status must only query system metadata through the default database")
		}
	})
	state, err := store.MigrationStatus(context.Background())
	if err != nil || state.Version != 0 || state.Target == 0 || len(state.Pending) == 0 || state.Dirty {
		t.Fatalf("fresh status: %+v, %v", state, err)
	}
}

func TestClickHouseMigrationJournalAndRerun(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("fail=%t", fail), func(t *testing.T) {
			metadata, clean, applied := false, false, 0
			var version uint
			schemaReads, columnReads := 0, 0
			store := testStore(t, func(w http.ResponseWriter, r *http.Request) {
				query := r.URL.Query().Get("query")
				switch {
				case strings.HasPrefix(query, "SELECT name FROM system.tables"):
					if metadata {
						fmt.Fprintln(w, `{"name":"tracepilot_schema_migrations"}`)
					}
				case strings.HasPrefix(query, "SELECT version,"):
					if version > 0 {
						fmt.Fprintf(w, "{\"version\":%d,\"dirty\":%t}\n", version, !clean)
					}
				case strings.HasPrefix(query, "CREATE TABLE IF NOT EXISTS tracepilot_schema_migrations"):
					metadata = true
				case strings.HasPrefix(query, "INSERT INTO tracepilot_schema_migrations"):
					if _, err := fmt.Sscanf(query, "INSERT INTO tracepilot_schema_migrations VALUES (%d,", &version); err != nil {
						t.Error(err)
					}
					if strings.Contains(query, "false") {
						clean = true
					}
				case strings.HasPrefix(query, "CREATE TABLE IF NOT EXISTS tracepilot_logs"):
					applied++
					if fail {
						w.WriteHeader(http.StatusInternalServerError)
					}
				case strings.HasPrefix(query, "SELECT engine,"):
					schemaReads++
					_ = json.NewEncoder(w).Encode(schemaState{Engine: "ReplacingMergeTree",
						Partition: "toStartOfWeek(occurred_at, 1, 'UTC')",
						Sorting:   "project_id, environment_id, service_name, toStartOfMillisecond(occurred_at), event_id"})
				case strings.HasPrefix(query, "SELECT "+logColumns):
					columnReads++
				}
			})
			err := store.Migrate(context.Background())
			if (err != nil) != fail || applied != 1 || clean == fail {
				t.Fatalf("migration journal: applied=%d clean=%t error=%v", applied, clean, err)
			}
			if !fail && (version != 2 || schemaReads != 2 || columnReads != 2) {
				t.Fatalf("migration repeated schema probes: schema=%d columns=%d", schemaReads, columnReads)
			}
			err = store.Migrate(context.Background())
			if (err != nil) != fail || applied != 1 {
				t.Fatalf("completed/dirty migration was replayed: applied=%d error=%v", applied, err)
			}
			if !fail && (schemaReads != 3 || columnReads != 3) {
				t.Fatalf("rerun repeated schema probes: schema=%d columns=%d", schemaReads, columnReads)
			}
			if err := store.Ready(context.Background()); (err != nil) != fail {
				t.Fatalf("readiness accepted wrong journal state: %v", err)
			}
		})
	}
}

func TestMigrationValidatesLegacyLayoutBeforeChangingTTL(t *testing.T) {
	started, clean, altered := false, false, false
	store := testStore(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		switch {
		case strings.HasPrefix(query, "INSERT INTO tracepilot_schema_migrations"):
			started = true
			clean = strings.Contains(query, "false")
		case strings.HasPrefix(query, "SELECT engine,"):
			_ = json.NewEncoder(w).Encode(schemaState{Engine: "MergeTree"})
		case strings.HasPrefix(query, "ALTER TABLE"):
			altered = true
		}
	})
	if err := store.Migrate(context.Background()); err == nil || !started || clean || altered {
		t.Fatalf("invalid layout was changed or marked clean: started=%t clean=%t altered=%t error=%v", started, clean, altered, err)
	}
}
