package rollups

// selectDirtyMinutesSQL lists closed minutes whose latest span mark is newer
// than their last rollup, oldest first. A minute never rolled up joins no done
// row and compares against the zero time.
const selectDirtyMinutesSQL = `
SELECT
    toString(marks.project_id) AS project,
    toString(marks.environment_id) AS environment,
    marks.minute AS minute
FROM (
    SELECT project_id, environment_id, minute, max(marked_at) AS marked
    FROM tracepilot_span_minute_marks
    WHERE minute >= {lookback:DateTime64(9, 'UTC')}
      AND minute < toStartOfMinute({now:DateTime64(9, 'UTC')})
    GROUP BY project_id, environment_id, minute
) AS marks
LEFT JOIN (
    SELECT project_id, environment_id, minute, max(computed_at) AS computed
    FROM tracepilot_span_minute_done
    WHERE minute >= {lookback:DateTime64(9, 'UTC')}
    GROUP BY project_id, environment_id, minute
) AS done
    ON marks.project_id = done.project_id
   AND marks.environment_id = done.environment_id
   AND marks.minute = done.minute
WHERE marks.marked > done.computed
ORDER BY minute
LIMIT {limit:UInt32}
FORMAT JSONEachRow`

// insertSpanRollupsSQL rebuilds whole minutes from deduplicated spans, so
// retries and late spans never double-count. Callers append client.FinalSettings.
const insertSpanRollupsSQL = `
INSERT INTO tracepilot_span_minutes
SELECT
    project_id,
    environment_id,
    toDateTime(toStartOfMinute(started_at), 'UTC') AS minute,
    service_name,
    span_kind,
    endpoint,
    dependency_type,
    dependency_name,
    count(),
    countIf(status_code = 'ERROR'),
    countIf(request_failed),
    sum(duration_ns),
    quantilesTDigestState(0.95, 0.99)(duration_ns),
    {computed:DateTime64(3, 'UTC')}
FROM tracepilot_spans FINAL
WHERE project_id = {project:UUID}
  AND environment_id = {environment:UUID}
  AND started_at >= {first:DateTime64(9, 'UTC')}
  AND started_at < {last:DateTime64(9, 'UTC')}
  AND has({minutes:Array(DateTime('UTC'))}, minute)
GROUP BY
    project_id, environment_id, minute, service_name,
    span_kind, endpoint, dependency_type, dependency_name`

// insertRollupCompletionSQL records when each minute's rollup was computed.
const insertRollupCompletionSQL = `
INSERT INTO tracepilot_span_minute_done
SELECT
    {project:UUID},
    {environment:UUID},
    arrayJoin({minutes:Array(DateTime('UTC'))}),
    {computed:DateTime64(3, 'UTC')}`
