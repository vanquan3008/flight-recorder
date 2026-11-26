UNION ALL
SELECT
    service_name,
    endpoint,
    toInt32(intDiv(toUnixTimestamp64Nano(toDateTime64(minute, 9, 'UTC')) - {from_ns:Int64}, {bucket_ns:Int64})),
    sum(calls),
    sum(failed),
    sum(duration_sum),
    quantilesTDigestMergeState(0.95, 0.99)(duration_quantiles)
FROM tracepilot_span_minutes FINAL
WHERE %s
  AND minute IN (SELECT minute FROM clean)
GROUP BY service_name, endpoint, 3
