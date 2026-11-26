package analytics

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry/telemetrytest"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/clienttest"
)

func TestPerformanceTemplatesPreserveScopeAndRollupBoundary(t *testing.T) {
	for _, window := range []time.Duration{30 * time.Minute, time.Hour, 2 * time.Hour} {
		t.Run(window.String(), func(t *testing.T) {
			from := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)
			filter := query.PerformanceFilter{
				EnvironmentID: telemetrytest.Environment, From: from, To: from.Add(window),
				Since: from.Add(17 * time.Second), Limit: 10,
				Service: "api' OR 1=1 -- 100%", Endpoint: "/orders?value='",
			}
			calls := 0
			store := New(clienttest.Client(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				values := r.URL.Query()
				sql := strings.Join(strings.Fields(values.Get("query")), " ")
				for _, term := range []string{
					"project_id = {project:UUID}", "environment_id = {environment:UUID}",
					"service_name = {service:String}", "endpoint = {endpoint:String}",
					"span_kind = 2", "started_at >= {from:DateTime64(9, 'UTC')}",
					"started_at < {to:DateTime64(9, 'UTC')}",
					"quantilesTDigestMerge(0.95, 0.99)(quantile_state)",
					"force_grouping_standard_compatibility = 1", "FORMAT JSONEachRow",
				} {
					if !strings.Contains(sql, term) {
						t.Errorf("missing %q in query: %s", term, sql)
					}
				}
				for name, want := range map[string]string{
					"project": telemetrytest.Project, "environment": filter.EnvironmentID,
					"service": filter.Service, "endpoint": filter.Endpoint,
					"from": client.Time(filter.Since), "to": client.Time(filter.To),
				} {
					if got := values.Get("param_" + name); got != want {
						t.Errorf("parameter %s = %q, want %q", name, got, want)
					}
				}
				if strings.Contains(sql, filter.Service) || strings.Contains(sql, filter.Endpoint) || strings.Contains(sql, "%!") || strings.Contains(sql, "%s") {
					t.Errorf("unbound value or invalid template in query: %s", sql)
				}
				rollup := window >= time.Hour
				if strings.Contains(sql, "WITH clean AS (") != rollup || strings.Contains(sql, "UNION ALL") != rollup {
					t.Errorf("unexpected rollup branch: %s", sql)
				}
				if rollup {
					for _, term := range []string{
						"GROUP BY service_name, endpoint, bucket UNION ALL SELECT",
						"NOT IN (SELECT minute FROM clean)",
						"FROM tracepilot_span_minutes FINAL", "minute IN (SELECT minute FROM clean)",
						"done.computed >= marks.marked",
					} {
						if !strings.Contains(sql, term) {
							t.Errorf("missing rollup clause %q: %s", term, sql)
						}
					}
					if values.Get("param_clean_from") != client.Time(filter.Since) || values.Get("param_clean_to") != client.Time(filter.To) {
						t.Error("clean minutes must respect retention and the window end")
					}
				}
				fmt.Fprintln(w, `{"dimension":"summary","requests":10,"errors":2,"average_ns":100,"quantiles":[150,200]}`)
			}))
			result, err := store.RequestPerformance(context.Background(), telemetrytest.Project, filter)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || result.Summary.Requests != 10 || result.Summary.ErrorRate != 0.2 || result.Summary.P99LatencyNanos != 200 {
				t.Fatalf("calls = %d, summary = %+v", calls, result.Summary)
			}
		})
	}
}
