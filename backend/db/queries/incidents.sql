-- name: ListIncidents :many
SELECT id, primary_service, COALESCE(trace_id, '')::text AS trace_id, COALESCE(request_id, '')::text AS request_id,
       fingerprint, title, COALESCE(error_type, '')::text AS error_type, COALESCE(error_message, '')::text AS error_message,
       http_status, severity, status, first_seen_at, last_seen_at, occurrence_count
FROM incidents
WHERE project_id = @project_id
  AND environment_id = @environment_id
  AND (sqlc.narg(service)::text IS NULL OR primary_service = sqlc.narg(service))
  AND (sqlc.narg(severity)::text IS NULL OR severity = sqlc.narg(severity))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR last_seen_at >= sqlc.narg(from_time))
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR last_seen_at <= sqlc.narg(to_time))
  AND (sqlc.narg(before)::timestamptz IS NULL
       OR last_seen_at < sqlc.narg(before)
       OR (last_seen_at = sqlc.narg(before) AND id < sqlc.narg(cursor_id)::uuid))
ORDER BY last_seen_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer;

-- name: GetIncident :one
SELECT id, primary_service, COALESCE(trace_id, '')::text AS trace_id, COALESCE(request_id, '')::text AS request_id,
       fingerprint, title, COALESCE(error_type, '')::text AS error_type, COALESCE(error_message, '')::text AS error_message,
       http_status, severity, status, first_seen_at, last_seen_at, occurrence_count
FROM incidents
WHERE project_id = @project_id AND environment_id = @environment_id AND id = @id;

-- name: ListIncidentOccurrences :many
SELECT occurrence.id, occurrence.event_id, occurrence.occurred_at, event.service_name,
       COALESCE(event.trace_id, '')::text AS trace_id, event.event_type,
       COALESCE(event.operation, '')::text AS operation, COALESCE(event.error_type, '')::text AS error_type,
       COALESCE(event.error_message, '')::text AS error_message, event.http_status
FROM incident_occurrences AS occurrence
JOIN incidents AS scoped_incident
  ON scoped_incident.project_id = occurrence.project_id
 AND scoped_incident.id = occurrence.incident_id
 AND scoped_incident.environment_id = @environment_id
JOIN events AS event
  ON event.project_id = occurrence.project_id
 AND event.id = occurrence.event_id
 AND event.environment_id = @environment_id
WHERE occurrence.project_id = @project_id
  AND occurrence.incident_id = @incident_id
  AND (sqlc.narg(before)::timestamptz IS NULL
       OR occurrence.occurred_at < sqlc.narg(before)
       OR (occurrence.occurred_at = sqlc.narg(before) AND occurrence.id < sqlc.narg(cursor_id)::uuid))
ORDER BY occurrence.occurred_at DESC, occurrence.id DESC
LIMIT sqlc.arg(row_limit)::integer;

-- name: ListIncidentEvents :many
SELECT event.id, event.occurred_at, event.service_name, COALESCE(event.trace_id, '')::text AS trace_id,
       event.event_type, COALESCE(event.operation, '')::text AS operation,
       COALESCE(event.error_type, '')::text AS error_type, COALESCE(event.error_message, '')::text AS error_message,
       event.http_status
FROM incident_events AS link
JOIN incidents AS scoped_incident
  ON scoped_incident.project_id = link.project_id
 AND scoped_incident.id = link.incident_id
 AND scoped_incident.environment_id = @environment_id
JOIN events AS event
  ON event.project_id = link.project_id
 AND event.id = link.event_id
 AND event.environment_id = @environment_id
WHERE link.project_id = @project_id
  AND link.incident_id = @incident_id
  AND (sqlc.narg(after)::timestamptz IS NULL
       OR event.occurred_at > sqlc.narg(after)
       OR (event.occurred_at = sqlc.narg(after) AND event.id > sqlc.narg(cursor_id)::uuid))
ORDER BY event.occurred_at ASC, event.id ASC
LIMIT sqlc.arg(row_limit)::integer;

-- name: ListIncidentStatusChanges :many
SELECT change.id, change.previous_status, change.status, change.actor_key_id, change.changed_at
FROM incident_status_changes AS change
JOIN incidents AS scoped_incident
  ON scoped_incident.project_id = change.project_id
 AND scoped_incident.id = change.incident_id
 AND scoped_incident.environment_id = @environment_id
WHERE change.project_id = @project_id
  AND change.incident_id = @incident_id
  AND (sqlc.narg(before)::timestamptz IS NULL
       OR change.changed_at < sqlc.narg(before)
       OR (change.changed_at = sqlc.narg(before) AND change.id < sqlc.narg(cursor_id)::uuid))
ORDER BY change.changed_at DESC, change.id DESC
LIMIT sqlc.arg(row_limit)::integer;

-- name: LockIncidentStatus :one
SELECT status FROM incidents
WHERE project_id = @project_id AND environment_id = @environment_id AND id = @id
FOR UPDATE;

-- name: SetIncidentStatus :exec
UPDATE incidents SET status = @status, updated_at = @changed_at
WHERE project_id = @project_id AND environment_id = @environment_id AND id = @id;

-- name: InsertIncidentStatusChange :exec
INSERT INTO incident_status_changes (id, project_id, incident_id, previous_status, status, actor_key_id, changed_at)
VALUES (@id, @project_id, @incident_id, @previous_status, @status, @actor_key_id, @changed_at);

-- Serializes writers of one fingerprint until the transaction ends.
-- name: LockIncidentFingerprint :exec
SELECT pg_advisory_xact_lock(hashtext(@lock_key::text));

-- name: ListRecordedOccurrenceEventIDs :many
SELECT event_id FROM incident_occurrences
WHERE project_id = @project_id AND event_id = ANY(@event_ids::uuid[]);

-- name: FindIncidentInWindow :one
SELECT id, last_seen_at FROM incidents
WHERE project_id = @project_id
  AND environment_id = @environment_id
  AND fingerprint = @fingerprint
  AND last_seen_at BETWEEN @window_start::timestamptz AND @window_end::timestamptz
ORDER BY last_seen_at DESC, id DESC
LIMIT 1;

-- name: InsertIncident :exec
INSERT INTO incidents (
    id, project_id, environment_id, primary_service, trace_id, request_id, fingerprint,
    title, error_type, error_message, http_status, severity, status,
    first_seen_at, last_seen_at, occurrence_count
) VALUES (
    @id, @project_id, @environment_id, @primary_service, sqlc.narg(trace_id), sqlc.narg(request_id), @fingerprint,
    @title, sqlc.narg(error_type), sqlc.narg(error_message), sqlc.narg(http_status), @severity, 'OPEN',
    @first_seen_at, @last_seen_at, @occurrence_count
);

-- Optional details keep their current value unless the batch carries a newer one.
-- name: AddIncidentOccurrences :exec
UPDATE incidents
SET first_seen_at = LEAST(first_seen_at, @first_seen_at::timestamptz),
    last_seen_at = GREATEST(last_seen_at, @last_seen_at::timestamptz),
    occurrence_count = occurrence_count + @occurrence_count::bigint,
    trace_id = COALESCE(sqlc.narg(trace_id), trace_id),
    request_id = COALESCE(sqlc.narg(request_id), request_id),
    error_type = COALESCE(sqlc.narg(error_type), error_type),
    error_message = COALESCE(sqlc.narg(error_message), error_message),
    http_status = COALESCE(sqlc.narg(http_status), http_status),
    severity = @severity,
    updated_at = now()
WHERE project_id = @project_id AND id = @id;

-- name: InsertIncidentOccurrence :exec
INSERT INTO incident_occurrences (id, project_id, incident_id, event_id, occurred_at)
VALUES (@id, @project_id, @incident_id, @event_id, @occurred_at);

-- name: LinkIncidentEvent :exec
INSERT INTO incident_events (project_id, incident_id, event_id)
VALUES (@project_id, @incident_id, @event_id)
ON CONFLICT DO NOTHING;

-- name: InsertEventEvidence :exec
INSERT INTO incident_evidence (
    id, project_id, environment_id, incident_id, signal_type, entity_type, entity_id, metric_name,
    direction, observed_value, baseline_value, change_ratio, window_started_at, window_ended_at,
    trace_ids, summary
) VALUES (
    @id, @project_id, @environment_id, @incident_id, 'event', 'service', @entity_id, 'error_event',
    'event', 1, 0, 1, @occurred_at, @occurred_at,
    @trace_ids, @summary
)
ON CONFLICT DO NOTHING;
