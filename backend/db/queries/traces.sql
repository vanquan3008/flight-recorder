-- name: ListTraceSpans :many
SELECT
    event_id::text AS event_id,
    trace_id,
    span_id,
    COALESCE(parent_span_id, '') AS parent_span_id,
    trace_state,
    span_name,
    span_kind,
    started_at,
    ended_at,
    status_code,
    status_message,
    flags,
    service_name,
    COALESCE(service_version, '') AS service_version,
    resource_attributes,
    scope_name,
    scope_version,
    scope_attributes,
    span_attributes,
    span_events,
    span_links
FROM spans
WHERE project_id = sqlc.arg(project_id)
  AND environment_id = sqlc.arg(environment_id)
  AND trace_id = sqlc.arg(trace_id)
ORDER BY started_at, event_id;
