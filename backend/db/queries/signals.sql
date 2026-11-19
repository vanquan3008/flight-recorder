-- Ingest writes. Every insert is idempotent so an exporter retry of the same
-- records cannot create duplicates.
--
-- A conflict target names one unique index. When a table has more than one, two
-- connections inserting the same row at once can collide on an index the target
-- does not name and fail with 23505 instead of skipping. Statements on such
-- tables therefore use a bare ON CONFLICT DO NOTHING.

-- name: InsertEvent :exec
INSERT INTO events (
    id, project_id, environment_id, service_name, service_version,
    trace_id, span_id, parent_span_id, request_id, event_type,
    occurred_at, duration_ns, severity, status, http_status,
    operation, error_type, error_message, attributes
) VALUES (
    @id, @project_id, @environment_id, @service_name, sqlc.narg(service_version),
    sqlc.narg(trace_id), sqlc.narg(span_id), sqlc.narg(parent_span_id), sqlc.narg(request_id), @event_type,
    @occurred_at, sqlc.narg(duration_ns), sqlc.narg(severity), sqlc.narg(status), sqlc.narg(http_status),
    sqlc.narg(operation), sqlc.narg(error_type), sqlc.narg(error_message), @attributes
)
ON CONFLICT (id, occurred_at) DO NOTHING;
