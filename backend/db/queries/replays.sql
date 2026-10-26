-- name: FailStaleReplayExecutions :execrows
UPDATE replay_executions
SET status = 'failed', completed_at = now(),
    comparison = '{"reason":"replay execution timed out before completion"}'::jsonb
WHERE status = 'started' AND started_at < @started_before::timestamptz;

-- The incident lookup scopes the replay to the caller's environment; no row is
-- inserted when the incident belongs elsewhere.
-- name: StartReplayExecution :execrows
INSERT INTO replay_executions (id, project_id, incident_id, target, status, started_at, created_by)
SELECT @id, @project_id, incidents.id, @target, 'started', now(), @created_by
FROM incidents
WHERE incidents.project_id = @project_id
  AND incidents.environment_id = @environment_id
  AND incidents.id = @incident_id;

-- name: CompleteReplayExecution :execrows
UPDATE replay_executions
SET target = @target, status = @status, completed_at = now(),
    duration_ns = sqlc.narg(duration_ns), http_status = sqlc.narg(http_status), comparison = @comparison
WHERE id = @id AND project_id = @project_id AND status = 'started';

-- name: ListReplayExecutions :many
SELECT execution.id, execution.incident_id, execution.target, execution.status,
       execution.started_at, execution.completed_at, execution.duration_ns, execution.http_status,
       COALESCE(execution.comparison, '{}'::jsonb)::jsonb AS comparison,
       execution.created_by, execution.created_at
FROM replay_executions AS execution
JOIN incidents
  ON incidents.project_id = execution.project_id AND incidents.id = execution.incident_id
WHERE execution.project_id = @project_id
  AND execution.incident_id = @incident_id
  AND incidents.environment_id = @environment_id
  AND (sqlc.narg(before)::timestamptz IS NULL
       OR execution.created_at < sqlc.narg(before)
       OR (execution.created_at = sqlc.narg(before) AND execution.id < sqlc.narg(cursor_id)::uuid))
ORDER BY execution.created_at DESC, execution.id DESC
LIMIT sqlc.arg(row_limit)::integer;

-- name: GetReplayExecution :one
SELECT execution.id, execution.incident_id, execution.target, execution.status,
       execution.started_at, execution.completed_at, execution.duration_ns, execution.http_status,
       COALESCE(execution.comparison, '{}'::jsonb)::jsonb AS comparison,
       execution.created_by, execution.created_at
FROM replay_executions AS execution
JOIN incidents
  ON incidents.project_id = execution.project_id AND incidents.id = execution.incident_id
WHERE execution.project_id = @project_id
  AND execution.incident_id = @incident_id
  AND execution.id = @id
  AND incidents.environment_id = @environment_id;
