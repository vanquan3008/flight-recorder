%s
SELECT
    multiIf(grouping(bucket) = 0, 'bucket', grouping(endpoint) = 0, 'endpoint', grouping(service_name) = 0, 'service', 'summary') AS dimension,
    service_name,
    endpoint,
    bucket,
    toInt64(sum(calls)) AS requests,
    toInt64(sum(failed)) AS errors,
    if(requests = 0, 0, toInt64(sum(duration_sum) / requests)) AS average_ns,
    quantilesTDigestMerge(0.95, 0.99)(quantile_state) AS quantiles
FROM (
%s
)
GROUP BY GROUPING SETS ((), (service_name), (service_name, endpoint), (bucket))
%s, force_grouping_standard_compatibility = 1
FORMAT JSONEachRow
