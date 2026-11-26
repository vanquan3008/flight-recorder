SELECT trace_id, max(started_at) AS last_match
FROM tracepilot_spans FINAL
WHERE %s
GROUP BY trace_id
