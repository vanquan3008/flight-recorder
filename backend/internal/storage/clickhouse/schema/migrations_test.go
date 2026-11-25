package schema

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/clienttest"

	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/logs"

	"github.com/vanquan3008/tracepilot/db"
)

func latestClickHouseVersion(t *testing.T) uint {
	t.Helper()
	list, err := db.List(db.ClickHouseMigrations)
	if err != nil {
		t.Fatal(err)
	}
	return list[len(list)-1].Version
}

func TestMigrationStatusIsReadOnlyForMissingDatabase(t *testing.T) {
	store := clienttest.Store(t, New, func(w http.ResponseWriter, r *http.Request) {
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
			metadata, clean, applied, columnReads := false, false, 0, 0
			var version uint
			store := clienttest.Store(t, New, func(w http.ResponseWriter, r *http.Request) {
				query := r.URL.Query().Get("query")
				switch {
				case strings.HasPrefix(query, "SELECT name, engine FROM system.tables"):
					fmt.Fprintln(w, `{"name":"tracepilot_trace_index_mv","engine":"MaterializedView"}
{"name":"tracepilot_span_minute_marks_mv","engine":"MaterializedView"}
{"name":"tracepilot_span_minutes","engine":"ReplicatedReplacingMergeTree"}
{"name":"tracepilot_span_minute_marks","engine":"ReplacingMergeTree"}
{"name":"tracepilot_span_minute_done","engine":"ReplacingMergeTree"}`)
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
					clean = strings.Contains(query, "false")
				case strings.HasPrefix(query, "CREATE TABLE IF NOT EXISTS tracepilot_log_records"):
					applied++
					if strings.Contains(query, "__PARTITION__") {
						t.Error("the partition placeholder reached ClickHouse")
					}
					if fail {
						w.WriteHeader(http.StatusInternalServerError)
					}
				case strings.HasPrefix(query, "SELECT "+logs.Columns):
					columnReads++
				}
			})
			err := store.Migrate(context.Background())
			if (err != nil) != fail || applied != 1 || clean == fail {
				t.Fatalf("migration journal: applied=%d clean=%t error=%v", applied, clean, err)
			}
			if !fail && (version != latestClickHouseVersion(t) || columnReads != 0) {
				t.Fatalf("first run: version=%d column checks=%d", version, columnReads)
			}
			err = store.Migrate(context.Background())
			if (err != nil) != fail || applied != 1 {
				t.Fatalf("completed/dirty migration was replayed: applied=%d error=%v", applied, err)
			}
			if !fail && columnReads != 1 {
				t.Fatalf("a run with nothing pending must check the current tables: %d checks", columnReads)
			}
			if err := store.Ready(context.Background()); (err != nil) != fail {
				t.Fatalf("readiness accepted wrong journal state: %v", err)
			}
		})
	}
}
