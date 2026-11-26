SELECT occurred_at, toString(event_id) AS id
FROM tracepilot_log_records FINAL
WHERE %s
ORDER BY occurred_at DESC, id DESC
LIMIT {limit:UInt32}%s
FORMAT JSONEachRow
