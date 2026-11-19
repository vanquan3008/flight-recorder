package clickhouse

import (
	"context"
	"strconv"
	"strings"
	"time"
)

const (
	// rollupLookback bounds the search for minutes that still need a rollup.
	rollupLookback = 8 * 24 * time.Hour
	// rollupBatch caps the minutes one refresh recomputes.
	rollupBatch = 1000
	// rollupSafety marks a refresh as computed slightly before it started, so a
	// span whose mark was taken while its insert was still committing makes its
	// minute dirty again instead of being missed.
	rollupSafety = 5 * time.Second
)

type dirtyMinute struct {
	Project     string    `json:"project"`
	Environment string    `json:"environment"`
	Minute      time.Time `json:"minute"`
}

type serverClock struct {
	Now time.Time `json:"server_now"`
}

// RefreshSpanRollups recomputes the minute rollups of closed minutes whose
// latest span arrived after their last rollup, oldest first, and returns how
// many minutes it recomputed. Each minute is rebuilt from deduplicated spans,
// so retries and late spans never double-count. Every timestamp comes from the
// ClickHouse clock, which also stamps the marks.
func (store *Store) RefreshSpanRollups(ctx context.Context) (int, error) {
	clock, err := queryRows[serverClock](ctx, store, "SELECT now64(3, 'UTC') AS server_now FORMAT JSONEachRow", nil)
	if err != nil || len(clock) == 0 {
		return 0, err
	}
	now := clock[0].Now
	parameters := map[string]string{"now": chTime(now), "lookback": chTime(now.Add(-rollupLookback)), "limit": strconv.Itoa(rollupBatch)}
	dirty, err := queryRows[dirtyMinute](ctx, store, `SELECT toString(marks.project_id) AS project, toString(marks.environment_id) AS environment,
marks.minute AS minute
FROM (SELECT project_id, environment_id, minute, max(marked_at) AS marked FROM tracepilot_span_minute_marks
  WHERE minute >= {lookback:DateTime64(9, 'UTC')} AND minute < toStartOfMinute({now:DateTime64(9, 'UTC')})
  GROUP BY project_id, environment_id, minute) AS marks
LEFT JOIN (SELECT project_id, environment_id, minute, max(computed_at) AS computed FROM tracepilot_span_minute_done
  WHERE minute >= {lookback:DateTime64(9, 'UTC')} GROUP BY project_id, environment_id, minute) AS done
ON marks.project_id = done.project_id AND marks.environment_id = done.environment_id AND marks.minute = done.minute
WHERE marks.marked > done.computed ORDER BY minute LIMIT {limit:UInt32} FORMAT JSONEachRow`, parameters)
	if err != nil || len(dirty) == 0 {
		return 0, err
	}
	computedAt := chTime(now.Add(-rollupSafety))
	scopes := map[[2]string][]time.Time{}
	for _, minute := range dirty {
		key := [2]string{minute.Project, minute.Environment}
		scopes[key] = append(scopes[key], minute.Minute)
	}
	for key, minutes := range scopes {
		if err := store.rollupMinutes(ctx, key[0], key[1], minutes, computedAt); err != nil {
			return 0, err
		}
	}
	return len(dirty), nil
}

func (store *Store) rollupMinutes(ctx context.Context, projectID, environmentID string, minutes []time.Time, computedAt string) error {
	if err := validateScope(projectID, environmentID); err != nil {
		return err
	}
	first, last := minutes[0], minutes[0]
	quoted := make([]string, len(minutes))
	for index, minute := range minutes {
		first, last = minTime(first, minute), maxTime(last, minute)
		quoted[index] = "'" + minute.UTC().Format("2006-01-02 15:04:05") + "'"
	}
	parameters := map[string]string{"project": projectID, "environment": environmentID, "computed": computedAt,
		"first": chTime(first), "last": chTime(last.Add(time.Minute)), "minutes": "[" + strings.Join(quoted, ",") + "]"}
	if _, err := store.execute(ctx, `INSERT INTO tracepilot_span_minutes
SELECT project_id, environment_id, toDateTime(toStartOfMinute(started_at), 'UTC') AS minute, service_name, span_kind, endpoint,
dependency_type, dependency_name, count(), countIf(status_code = 'ERROR'), countIf(request_failed), sum(duration_ns),
quantilesTDigestState(0.95, 0.99)(duration_ns), {computed:DateTime64(3, 'UTC')}
FROM tracepilot_spans FINAL WHERE project_id = {project:UUID} AND environment_id = {environment:UUID}
AND started_at >= {first:DateTime64(9, 'UTC')} AND started_at < {last:DateTime64(9, 'UTC')} AND has({minutes:Array(DateTime('UTC'))}, minute)
GROUP BY project_id, environment_id, minute, service_name, span_kind, endpoint, dependency_type, dependency_name`+finalSettings, parameters, nil, false); err != nil {
		return err
	}
	_, err := store.execute(ctx, `INSERT INTO tracepilot_span_minute_done
SELECT {project:UUID}, {environment:UUID}, arrayJoin({minutes:Array(DateTime('UTC'))}), {computed:DateTime64(3, 'UTC')}`, parameters, nil, false)
	return err
}

func minTime(first, second time.Time) time.Time {
	if second.Before(first) {
		return second
	}
	return first
}

func maxTime(first, second time.Time) time.Time {
	if second.After(first) {
		return second
	}
	return first
}
