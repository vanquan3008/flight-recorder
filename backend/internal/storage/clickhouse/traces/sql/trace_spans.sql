SELECT %s
FROM tracepilot_spans FINAL
WHERE %s
ORDER BY started_at, toString(event_id)
LIMIT {limit:UInt32}%s
FORMAT JSONEachRow
