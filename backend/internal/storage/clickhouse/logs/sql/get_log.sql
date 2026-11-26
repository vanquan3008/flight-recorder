SELECT %s
FROM tracepilot_log_records FINAL
WHERE %s
LIMIT 1%s
FORMAT JSONEachRow
