SELECT
    min(started_at) AS first_started,
    max(ended_at) AS last_ended
FROM tracepilot_trace_index
WHERE %s
HAVING count() > 0
FORMAT JSONEachRow
