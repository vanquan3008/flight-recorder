SELECT
    count() AS logs,
    countIf(trace_id = '') AS uncorrelated
FROM tracepilot_log_records FINAL
WHERE %s%s
FORMAT JSONEachRow
