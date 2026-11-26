INSERT INTO tracepilot_span_minutes
SELECT
    project_id,
    environment_id,
    toDateTime(toStartOfMinute(started_at), 'UTC') AS minute,
    service_name,
    span_kind,
    endpoint,
    dependency_type,
    dependency_name,
    count(),
    countIf(status_code = 'ERROR'),
    countIf(request_failed),
    sum(duration_ns),
    quantilesTDigestState(0.95, 0.99)(duration_ns),
    {computed:DateTime64(3, 'UTC')}
FROM tracepilot_spans FINAL
WHERE project_id = {project:UUID}
  AND environment_id = {environment:UUID}
  AND started_at >= {first:DateTime64(9, 'UTC')}
  AND started_at < {last:DateTime64(9, 'UTC')}
  AND has({minutes:Array(DateTime('UTC'))}, minute)
GROUP BY
    project_id, environment_id, minute, service_name,
    span_kind, endpoint, dependency_type, dependency_name
