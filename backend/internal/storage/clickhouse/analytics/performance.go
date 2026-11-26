package analytics

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"
)

// rollupMinimumWindow is the shortest window that reads minute rollups; shorter
// windows have buckets under a minute and read spans directly.
const rollupMinimumWindow = time.Hour

type performanceRow struct {
	Dimension string    `json:"dimension"`
	Service   string    `json:"service_name"`
	Endpoint  string    `json:"endpoint"`
	Bucket    int32     `json:"bucket"`
	Requests  int64     `json:"requests"`
	Errors    int64     `json:"errors"`
	Average   int64     `json:"average_ns"`
	Quantiles []float64 `json:"quantiles"`
}

// RequestPerformance summarizes inbound (SERVER) spans: the window, each
// service, each endpoint and each time bucket. Long windows read clean minute
// rollups and only the remaining minutes from spans; percentiles merge t-digest
// states from both, so they are approximate.
func (store *Analytics) RequestPerformance(ctx context.Context, projectID string, filter query.PerformanceFilter) (query.RequestPerformance, error) {
	if err := filter.Validate(); err != nil {
		return query.RequestPerformance{}, err
	}
	result := query.NewRequestPerformance(filter)
	where, parameters, err := client.Scope(projectID, filter.EnvironmentID)
	if err != nil {
		return result, err
	}
	parameters["from_ns"] = strconv.FormatInt(filter.From.UnixNano(), 10)
	parameters["bucket_ns"] = strconv.FormatInt(int64(result.BucketSeconds)*int64(time.Second), 10)
	where = client.AddEquals(where, parameters, [3]string{"service_name", "service", filter.Service}, [3]string{"endpoint", "endpoint", filter.Endpoint})
	from := client.MaxTime(filter.From, filter.Since)
	spans := client.AddTime(append(append([]string{}, where...), "span_kind = 2"), parameters, "started_at", "from", ">=", from)
	spans = client.AddTime(spans, parameters, "started_at", "to", "<", filter.To)
	rollups := append(append([]string{}, where...), "span_kind = 2")
	prefix, exclusion, rollupSource := "", "", ""
	if filter.To.Sub(filter.From) >= rollupMinimumWindow {
		prefix = "WITH clean AS (" + cleanMinutes(from, filter.To, parameters) + ")\n"
		exclusion = "AND toDateTime(toStartOfMinute(started_at), 'UTC') NOT IN (SELECT minute FROM clean)"
		rollupSource = fmt.Sprintf(performanceRollupsSQL, strings.Join(rollups, " AND "))
	}
	source := fmt.Sprintf(performanceSpansSQL, strings.Join(spans, " AND "), exclusion) + rollupSource
	statement := fmt.Sprintf(requestPerformanceSQL, prefix, source, client.FinalSettings)
	rows, err := client.QueryRows[performanceRow](ctx, store.client, statement, parameters)
	if err != nil {
		return result, fmt.Errorf("query request performance: %w", err)
	}
	seconds := filter.To.Sub(filter.From).Seconds()
	for _, row := range rows {
		stats := query.PerformanceStats{Requests: row.Requests, Errors: row.Errors, AverageLatencyNanos: row.Average}
		if len(row.Quantiles) == 2 {
			stats.P95LatencyNanos, stats.P99LatencyNanos = int64(row.Quantiles[0]), int64(row.Quantiles[1])
		}
		stats.SetRates(seconds)
		switch row.Dimension {
		case "summary":
			result.Summary = stats
		case "service":
			result.Services = append(result.Services, query.PerformanceGroup{Service: row.Service, PerformanceStats: stats})
		case "endpoint":
			result.Endpoints = append(result.Endpoints, query.PerformanceGroup{Service: row.Service, Endpoint: row.Endpoint, PerformanceStats: stats})
		case "bucket":
			if row.Bucket < 0 || int(row.Bucket) >= len(result.Series) {
				continue // a rollup minute at the edge of the last bucket
			}
			interval := &result.Series[row.Bucket]
			stats.SetRates(interval.To.Sub(interval.From).Seconds())
			interval.PerformanceStats = stats
		}
	}
	result.Services = topGroups(result.Services, filter.Limit)
	result.Endpoints = topGroups(result.Endpoints, filter.Limit)
	return result, nil
}

// cleanMinutes selects the minutes inside the window whose rollup was computed
// after their latest span arrived.
func cleanMinutes(from, to time.Time, parameters map[string]string) string {
	parameters["clean_from"], parameters["clean_to"] = client.Time(from), client.Time(to)
	return cleanMinutesSQL
}

func topGroups(groups []query.PerformanceGroup, limit int) []query.PerformanceGroup {
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Requests != groups[j].Requests {
			return groups[i].Requests > groups[j].Requests
		}
		if groups[i].Service != groups[j].Service {
			return groups[i].Service < groups[j].Service
		}
		return groups[i].Endpoint < groups[j].Endpoint
	})
	if len(groups) > limit {
		groups = groups[:limit]
	}
	return groups
}
