package analytics

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"

	"github.com/vanquan3008/tracepilot/internal/query"
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
	source := `
    SELECT
        service_name,
        endpoint,
        toInt32(intDiv(toUnixTimestamp64Nano(started_at) - {from_ns:Int64}, {bucket_ns:Int64})) AS bucket,
        count() AS calls,
        countIf(request_failed) AS failed,
        sum(duration_ns) AS duration_sum,
        quantilesTDigestState(0.95, 0.99)(duration_ns) AS quantile_state
    FROM tracepilot_spans FINAL
    WHERE ` + strings.Join(spans, " AND ")
	prefix := ""
	if filter.To.Sub(filter.From) >= rollupMinimumWindow {
		prefix = "WITH clean AS (" + cleanMinutes(from, filter.To, parameters) + ")\n"
		source += `
      AND toDateTime(toStartOfMinute(started_at), 'UTC') NOT IN (SELECT minute FROM clean)
    GROUP BY service_name, endpoint, bucket
    UNION ALL
    SELECT
        service_name,
        endpoint,
        toInt32(intDiv(toUnixTimestamp64Nano(toDateTime64(minute, 9, 'UTC')) - {from_ns:Int64}, {bucket_ns:Int64})),
        sum(calls),
        sum(failed),
        sum(duration_sum),
        quantilesTDigestMergeState(0.95, 0.99)(duration_quantiles)
    FROM tracepilot_span_minutes FINAL
    WHERE ` + strings.Join(rollups, " AND ") + `
      AND minute IN (SELECT minute FROM clean)
    GROUP BY service_name, endpoint, 3`
	} else {
		source += `
    GROUP BY service_name, endpoint, bucket`
	}
	rows, err := client.QueryRows[performanceRow](ctx, store.client, prefix+`SELECT
    multiIf(grouping(bucket) = 0, 'bucket', grouping(endpoint) = 0, 'endpoint', grouping(service_name) = 0, 'service', 'summary') AS dimension,
    service_name,
    endpoint,
    bucket,
    toInt64(sum(calls)) AS requests,
    toInt64(sum(failed)) AS errors,
    if(requests = 0, 0, toInt64(sum(duration_sum) / requests)) AS average_ns,
    quantilesTDigestMerge(0.95, 0.99)(quantile_state) AS quantiles
FROM (`+source+`
)
GROUP BY GROUPING SETS ((), (service_name), (service_name, endpoint), (bucket))`+client.FinalSettings+
		`, force_grouping_standard_compatibility = 1
FORMAT JSONEachRow`, parameters)
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
	window := `project_id = {project:UUID}
          AND environment_id = {environment:UUID}
          AND minute >= {clean_from:DateTime64(9, 'UTC')}
          AND minute + INTERVAL 1 MINUTE <= {clean_to:DateTime64(9, 'UTC')}`
	return `
    SELECT marks.minute AS minute
    FROM (
        SELECT minute, max(marked_at) AS marked
        FROM tracepilot_span_minute_marks
        WHERE ` + window + `
        GROUP BY minute
    ) AS marks
    INNER JOIN (
        SELECT minute, max(computed_at) AS computed
        FROM tracepilot_span_minute_done
        WHERE ` + window + `
        GROUP BY minute
    ) AS done ON marks.minute = done.minute
    WHERE done.computed >= marks.marked
`
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
