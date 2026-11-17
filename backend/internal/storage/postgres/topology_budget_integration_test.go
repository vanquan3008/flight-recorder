package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/query"
)

func TestExplorerFitsQueryBudgetForNewScope(t *testing.T) {
	database, baseline := newFlowTestDatabase(t)
	at := time.Now().UTC().Truncate(time.Second)
	if err := database.EnsurePartitions(context.Background(), at.AddDate(0, 0, -1), 10); err != nil {
		t.Fatal(err)
	}
	// Analyze an existing scope, then ingest into a new one absent from those statistics.
	// Autovacuum does not immediately learn a newly created project's cardinality.
	seed := func(project, environment string) {
		t.Helper()
		_, err := database.pool.Exec(context.Background(), `INSERT INTO spans (
event_id, project_id, environment_id, trace_id, span_id, parent_span_id,
span_name, span_kind, started_at, ended_at, status_code, service_name)
SELECT gen_random_uuid(), $1, $2, lpad(to_hex(n / 8 + 1), 32, '0'),
lpad(to_hex(n % 8 + 1), 16, '0'),
CASE WHEN n % 8 = 0 THEN NULL ELSE lpad(to_hex(n % 8), 16, '0') END,
'work', 1, $3::timestamptz, $3::timestamptz + interval '1 millisecond', 'OK',
CASE WHEN n % 2 = 0 THEN 'api' ELSE 'worker' END
	FROM generate_series(0, 15999) AS n`, project, environment, at)
		if err != nil {
			t.Fatal(err)
		}
	}
	seed(baseline.ProjectID, baseline.EnvironmentID)
	var partition string
	if err := database.pool.QueryRow(context.Background(), "SELECT tableoid::regclass::text FROM spans WHERE project_id=$1 LIMIT 1", baseline.ProjectID).Scan(&partition); err != nil {
		t.Fatal(err)
	}
	// Autovacuum analyzes leaf partitions, not their parent. Preserve that production condition.
	if _, err := database.pool.Exec(context.Background(), "ANALYZE "+pgx.Identifier{partition}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	_, scope := newFlowTestDatabase(t)
	seed(scope.ProjectID, scope.EnvironmentID)
	t.Run("topology", func(t *testing.T) {
		ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		topology, err := database.ListTopology(ctx, scope.ProjectID, analysis.Filter{
			EnvironmentID: scope.EnvironmentID, From: at.Add(-time.Minute), To: at.Add(time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
		var calls int64
		for _, edge := range topology.Edges {
			calls += edge.CallCount
		}
		if len(topology.Nodes) != 2 || len(topology.Edges) != 2 || calls != 14000 {
			t.Fatalf("nodes=%d edges=%d calls=%d, want 2/2/14000", len(topology.Nodes), len(topology.Edges), calls)
		}
	})
	t.Run("traces", func(t *testing.T) {
		ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		traces, err := database.ListTraces(ctx, scope.ProjectID, query.TraceFilter{
			EnvironmentID: scope.EnvironmentID, From: at.Add(-time.Minute), To: at.Add(time.Minute), Limit: 50,
		})
		if err != nil {
			t.Fatal(err)
		}
		// The storage contract includes one extra row for the next-page cursor.
		if len(traces) != 51 {
			t.Fatalf("traces=%d, want 51", len(traces))
		}
		for _, trace := range traces {
			if trace.SpanCount != 8 || trace.DurationNanos != int64(time.Millisecond) {
				t.Fatalf("partial trace: %#v", trace)
			}
		}
	})
}
