SELECT
    series_id,
    service_name,
    metric_name,
    metric_unit,
    metric_type,
    is_monotonic,
    temporality,
    attributes,
    occurred_at,
    started_at,
    value,
    hist_count,
    hist_sum
FROM tracepilot_metric_points FINAL
WHERE %s
  AND bitAnd(flags, 1) = 0
ORDER BY series_id, occurred_at, id%s
FORMAT JSONEachRow
