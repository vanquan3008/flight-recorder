CREATE MATERIALIZED VIEW IF NOT EXISTS tracepilot_trace_index_mv TO tracepilot_trace_index AS
SELECT project_id, environment_id, trace_id, min(started_at) AS started_at, max(ended_at) AS ended_at
FROM tracepilot_spans
GROUP BY project_id, environment_id, trace_id
