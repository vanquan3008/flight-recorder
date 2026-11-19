CREATE MATERIALIZED VIEW IF NOT EXISTS tracepilot_span_minute_marks_mv TO tracepilot_span_minute_marks AS
SELECT project_id, environment_id, toDateTime(toStartOfMinute(started_at), 'UTC') AS minute, max(now64(3, 'UTC')) AS marked_at
FROM tracepilot_spans
GROUP BY project_id, environment_id, minute
