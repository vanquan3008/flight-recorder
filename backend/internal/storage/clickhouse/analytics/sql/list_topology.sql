SELECT
    source_name,
    target_type,
    target_name,
    edge_kind,
    count() AS call_count,
    countIf(status_code = 'ERROR') AS error_count,
    toInt64(avg(duration_ns)) AS average_latency_nanos,
    toInt64(quantileTDigest(0.95)(duration_ns)) AS p95_latency_nanos,
    arraySlice(arraySort(groupUniqArray(trace_id)), 1, 5) AS trace_ids
FROM (
    SELECT
        parent.service_name AS source_name,
        'service' AS target_type,
        child.service_name AS target_name,
        'service' AS edge_kind,
        child.trace_id AS trace_id,
        child.status_code AS status_code,
        child.duration_ns AS duration_ns
    FROM (
        SELECT trace_id, parent_span_id, service_name, status_code, duration_ns
        FROM tracepilot_spans FINAL
        WHERE %s
          AND parent_span_id != ''
    ) AS child
    INNER JOIN (
        SELECT trace_id, span_id, service_name
        FROM tracepilot_spans FINAL
        WHERE %s
    ) AS parent
        ON child.trace_id = parent.trace_id
       AND child.parent_span_id = parent.span_id
    WHERE parent.service_name != child.service_name

    UNION ALL

    SELECT service_name, dependency_type, dependency_name, dependency_type, trace_id, status_code, duration_ns
    FROM tracepilot_spans FINAL
    WHERE %s
      AND dependency_type != ''
      AND dependency_name != ''
)%s
GROUP BY source_name, target_type, target_name, edge_kind
ORDER BY source_name, target_type, target_name%s
FORMAT JSONEachRow
