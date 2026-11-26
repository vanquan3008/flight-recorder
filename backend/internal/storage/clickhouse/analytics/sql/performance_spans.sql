SELECT
    service_name,
    endpoint,
    toInt32(intDiv(toUnixTimestamp64Nano(started_at) - {from_ns:Int64}, {bucket_ns:Int64})) AS bucket,
    count() AS calls,
    countIf(request_failed) AS failed,
    sum(duration_ns) AS duration_sum,
    quantilesTDigestState(0.95, 0.99)(duration_ns) AS quantile_state
FROM tracepilot_spans FINAL
WHERE %s
  %s
GROUP BY service_name, endpoint, bucket
