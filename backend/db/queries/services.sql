-- name: ListServices :many
SELECT
    service_name AS name,
    count(DISTINCT trace_id) AS trace_count,
    count(*) FILTER (WHERE span_kind = 2) AS request_count,
    count(*) FILTER (WHERE status_code = 'ERROR') AS error_count,
    COALESCE(
        count(*) FILTER (WHERE status_code = 'ERROR')::double precision /
            NULLIF(count(*), 0),
        0::double precision
    )::double precision AS error_rate,
    COALESCE(
        avg(extract(epoch FROM ended_at - started_at) * 1000000000)::bigint,
        0::bigint
    )::bigint AS average_latency
FROM spans
WHERE project_id = $1
  AND environment_id = $2
  AND (NOT sqlc.arg(has_from)::boolean OR started_at >= sqlc.arg(from_time)::timestamptz)
  AND (NOT sqlc.arg(has_to)::boolean OR started_at <= sqlc.arg(to_time)::timestamptz)
GROUP BY service_name
ORDER BY service_name;

-- name: ListServiceDependencies :many
SELECT
    child.service_name,
    count(*) AS call_count
FROM spans AS child
JOIN spans AS parent
    ON parent.project_id = child.project_id
   AND parent.trace_id = child.trace_id
   AND parent.span_id = child.parent_span_id
WHERE child.project_id = $1
  AND child.environment_id = $2
  AND parent.environment_id = $2
  AND parent.service_name = $3
  AND parent.service_name <> child.service_name
GROUP BY child.service_name
ORDER BY call_count DESC, child.service_name;
