SELECT
    toString(id) AS id,
    metric_name,
    metric_unit,
    metric_type,
    service_name,
    occurred_at,
    value,
    hist_count,
    hist_sum,
    attributes,
    exemplars
FROM tracepilot_metric_points FINAL
WHERE %s
ORDER BY occurred_at DESC, id DESC
LIMIT {limit:UInt32}%s
FORMAT JSONEachRow
