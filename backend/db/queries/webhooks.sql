-- name: InsertWebhook :exec
INSERT INTO webhooks (id, project_id, name, url, secret, events, enabled, created_at, updated_at)
VALUES (@id, @project_id, @name, @url, @secret, @events, true, @created_at, @created_at);

-- A NULL enabled keeps the current value, so renaming does not toggle delivery.
-- name: UpdateWebhook :one
UPDATE webhooks
SET name = @name, url = @url, events = @events,
    enabled = COALESCE(sqlc.narg(enabled)::boolean, enabled), updated_at = now()
WHERE project_id = @project_id AND id = @id
RETURNING id, name, url, events, enabled, created_at, updated_at;

-- Secret material is never selected for management responses.
-- name: ListWebhooks :many
SELECT id, name, url, events, enabled, created_at, updated_at
FROM webhooks
WHERE project_id = @project_id
  AND (sqlc.narg(before)::timestamptz IS NULL
       OR created_at < sqlc.narg(before)
       OR (created_at = sqlc.narg(before) AND id < sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer;

-- name: GetWebhook :one
SELECT id, name, url, events, enabled, created_at, updated_at
FROM webhooks
WHERE project_id = @project_id AND id = @id;

-- Delivery history cascades with the webhook because it holds outbound payloads.
-- name: DeleteWebhook :execrows
DELETE FROM webhooks WHERE project_id = @project_id AND id = @id;

-- name: RotateWebhookSecret :execrows
UPDATE webhooks SET secret = @secret, updated_at = now()
WHERE project_id = @project_id AND id = @id;

-- name: ListWebhookDeliveries :many
SELECT id, webhook_id, event_type, status, attempt, max_attempts, http_status,
       COALESCE(last_error, '')::text AS last_error, next_attempt_at, created_at
FROM webhook_deliveries
WHERE project_id = @project_id
  AND (sqlc.narg(webhook_id)::uuid IS NULL OR webhook_id = sqlc.narg(webhook_id))
  AND (sqlc.narg(before)::timestamptz IS NULL
       OR created_at < sqlc.narg(before)
       OR (created_at = sqlc.narg(before) AND id < sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer;
