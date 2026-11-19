-- Serializes work across replicas until the transaction ends: partition DDL and
-- startup provisioning, each under its own key.
-- name: AcquireTransactionLock :exec
SELECT pg_advisory_xact_lock(@lock_key::bigint);

-- name: ListChildPartitions :many
SELECT child.relname::text AS name
FROM pg_catalog.pg_inherits
JOIN pg_catalog.pg_class AS child ON child.oid = pg_inherits.inhrelid
JOIN pg_catalog.pg_class AS parent ON parent.oid = pg_inherits.inhparent
WHERE parent.relname = @parent_table::text
  AND parent.relnamespace = current_schema()::regnamespace;

-- name: CountEventsBefore :one
SELECT count(*) FROM events WHERE occurred_at < @before::timestamptz;

-- Retention for operational history. Each statement removes one bounded batch;
-- the caller repeats until a batch comes back short. Child rows (occurrences,
-- evidence, event links, status history, replay results) go with their incident
-- through ON DELETE CASCADE, so no orphan can remain.

-- Only incidents nobody is working on expire: an open or investigating incident
-- stays however old it is, and a resolved one keeps its full history for the
-- whole window after its last activity.
-- name: DeleteExpiredIncidents :execrows
DELETE FROM incidents
WHERE id IN (
    SELECT expired.id FROM incidents AS expired
    WHERE expired.status IN ('RESOLVED', 'IGNORED')
      AND expired.last_seen_at < @before::timestamptz
      AND expired.updated_at < @before::timestamptz
    ORDER BY expired.last_seen_at
    LIMIT @batch_size::integer
);

-- name: DeleteExpiredAuditEvents :execrows
DELETE FROM audit_events
WHERE id IN (
    SELECT expired.id FROM audit_events AS expired
    WHERE expired.created_at < @before::timestamptz
    ORDER BY expired.created_at
    LIMIT @batch_size::integer
);

-- Pending and sending deliveries are live work and never expire.
-- name: DeleteExpiredWebhookDeliveries :execrows
DELETE FROM webhook_deliveries
WHERE id IN (
    SELECT expired.id FROM webhook_deliveries AS expired
    WHERE expired.status IN ('succeeded', 'failed')
      AND expired.updated_at < @before::timestamptz
    ORDER BY expired.updated_at
    LIMIT @batch_size::integer
);
-- name: ListProjectRetention :many
SELECT id, telemetry_retention_days FROM projects ORDER BY id;

-- name: LockProjectRetentionCatalog :exec
LOCK TABLE projects IN SHARE MODE;

-- name: LockProjectRetention :one
SELECT telemetry_retention_days FROM projects WHERE id = @project_id FOR NO KEY UPDATE;

-- tableoid and ctid together identify a tuple across the daily partitions.
-- name: DeleteProjectEventsBefore :execrows
DELETE FROM events AS target USING (
    SELECT e.tableoid, e.ctid FROM events AS e
    WHERE e.project_id = @project_id AND e.occurred_at < @before_time
    LIMIT @batch_size::integer
) AS expired WHERE target.tableoid = expired.tableoid AND target.ctid = expired.ctid;

-- name: CountProjectEventsBefore :one
SELECT count(*) FROM events WHERE project_id = @project_id AND occurred_at < @before_time;
