SELECT %s
FROM tracepilot_log_records FINAL
WHERE %s
ORDER BY occurred_at DESC, toString(event_id) DESC
LIMIT {limit:UInt32}%s
FORMAT JSONEachRow
