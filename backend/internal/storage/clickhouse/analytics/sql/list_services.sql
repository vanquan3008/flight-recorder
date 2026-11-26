SELECT
    service_name AS name,
    uniqExact(trace_id) AS trace_count,
    countIf(span_kind = 2) AS request_count,
    countIf(status_code = 'ERROR') AS error_count,
    if(count() = 0, 0, error_count / count()) AS error_rate,
    toInt64(avg(duration_ns)) AS average_latency
FROM tracepilot_spans FINAL
WHERE %s
GROUP BY service_name
ORDER BY service_name%s
FORMAT JSONEachRow
