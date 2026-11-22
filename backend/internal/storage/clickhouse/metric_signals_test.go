package clickhouse

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestMetricDetectionStreamsLargeResultsWithoutAcceptingPartialEvidence(t *testing.T) {
	end := time.Now().UTC().Truncate(time.Minute)
	for _, truncated := range []bool{false, true} {
		name := "large response"
		if truncated {
			name = "truncated response"
		}
		t.Run(name, func(t *testing.T) {
			store := testStore(t, func(w http.ResponseWriter, _ *http.Request) {
				encoder := json.NewEncoder(w)
				for _, series := range []string{"a", "b"} {
					if series == "b" && !truncated {
						// Put more than 32 MiB between two anomalous series.
						row, err := json.Marshal(metricSample{SeriesID: "az", Metric: "other", Type: "gauge", OccurredAt: end.Add(-time.Minute),
							Attributes: map[string]string{"label": strings.Repeat("x", 8<<10)}})
						if err != nil {
							t.Error(err)
							return
						}
						row = append(row, '\n')
						for range maxResponseBytes/len(row) + 1 {
							if _, err := w.Write(row); err != nil {
								return
							}
						}
					}
					for i, age := range []time.Duration{30, 20, 4, 2} {
						value := []float64{1, 1, 3, 3}[i]
						if err := encoder.Encode(metricSample{SeriesID: series, Service: "worker", Metric: "queue.backlog",
							Type: "gauge", OccurredAt: end.Add(-age * time.Minute), Value: &value}); err != nil {
							return
						}
					}
				}
				if truncated {
					_, _ = io.WriteString(w, `{"series_id":`)
				}
			})
			store.client.Timeout = 30 * time.Second
			evidence, err := store.DetectMetricEvidence(context.Background(), detectionScope, end)
			if truncated {
				if err == nil || evidence != nil {
					t.Fatalf("truncated results returned partial evidence: %+v, %v", evidence, err)
				}
				return
			}
			if err != nil || len(evidence) != 2 || evidence[0].Item.SeriesID != "a" || evidence[1].Item.SeriesID != "b" {
				t.Fatalf("large result lost evidence: %+v, %v", evidence, err)
			}
		})
	}
}
