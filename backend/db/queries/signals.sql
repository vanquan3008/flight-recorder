-- Ingest writes. Every insert is idempotent so an exporter retry of the same
-- records cannot create duplicates.
--
-- A conflict target names one unique index. When a table has more than one, two
-- connections inserting the same row at once can collide on an index the target
-- does not name and fail with 23505 instead of skipping. Statements on such
-- tables therefore use a bare ON CONFLICT DO NOTHING.

-- name: EnsureService :exec
INSERT INTO services (id, project_id, environment_id, name)
VALUES (@id, @project_id, @environment_id, @name)
ON CONFLICT DO NOTHING;

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

-- name: InsertSpan :exec
INSERT INTO spans (
    event_id, project_id, environment_id, trace_id, span_id, parent_span_id,
    trace_state, span_name, span_kind, started_at, ended_at, status_code, status_message, flags,
    service_name, service_version, resource_schema_url, resource_attributes,
    scope_name, scope_version, scope_schema_url, scope_attributes, span_attributes,
    span_events, span_links, dropped_attributes_count, dropped_events_count, dropped_links_count
) VALUES (
    @event_id, @project_id, @environment_id, @trace_id, @span_id, sqlc.narg(parent_span_id),
    @trace_state, @span_name, @span_kind, @started_at, @ended_at, @status_code, @status_message, @flags,
    @service_name, sqlc.narg(service_version), @resource_schema_url, @resource_attributes,
    @scope_name, @scope_version, @scope_schema_url, @scope_attributes, @span_attributes,
    @span_events, @span_links, @dropped_attributes_count, @dropped_events_count, @dropped_links_count
)
ON CONFLICT DO NOTHING;

-- name: InsertMetricPoint :exec
INSERT INTO metric_data_points (
    id, project_id, environment_id, metric_name, metric_unit, metric_type,
    service_name, service_version, occurred_at,
    value, hist_count, hist_sum, hist_min, hist_max, hist_buckets,
    temporality, is_monotonic, attributes, resource_attributes,
    scope_name, scope_version, exemplars, started_at, flags, scope_attributes
) VALUES (
    @id, @project_id, @environment_id, @metric_name, @metric_unit, @metric_type,
    @service_name, sqlc.narg(service_version), @occurred_at,
    sqlc.narg(value), sqlc.narg(hist_count), sqlc.narg(hist_sum), sqlc.narg(hist_min), sqlc.narg(hist_max), sqlc.narg(hist_buckets),
    @temporality, sqlc.narg(is_monotonic), @attributes, @resource_attributes,
    @scope_name, @scope_version, @exemplars, sqlc.narg(started_at), @flags, @scope_attributes
)
ON CONFLICT (project_id, id, occurred_at) DO NOTHING;
