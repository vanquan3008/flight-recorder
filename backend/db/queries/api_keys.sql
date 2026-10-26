-- name: InsertAPIKey :exec
INSERT INTO api_keys (id, project_id, name, key_hash, scopes, expires_at, created_at)
VALUES (@id, @project_id, @name, @key_hash, @scopes, sqlc.narg(expires_at), @created_at);

-- name: ListAPIKeys :many
SELECT id, name, scopes, expires_at, created_at, revoked_at
FROM api_keys
WHERE project_id = @project_id
  AND (sqlc.narg(before)::timestamptz IS NULL
       OR created_at < sqlc.narg(before)
       OR (created_at = sqlc.narg(before) AND id < sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer;

-- name: ListAPIKeyEvents :many
SELECT id, api_key_id, actor_key_id, action, created_at
FROM api_key_events
WHERE project_id = @project_id
  AND (sqlc.narg(before)::timestamptz IS NULL
       OR created_at < sqlc.narg(before)
       OR (created_at = sqlc.narg(before) AND id < sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer;

-- name: LockActiveAPIKey :one
SELECT id, name, scopes, expires_at, created_at, revoked_at
FROM api_keys
WHERE project_id = @project_id AND id = @id AND revoked_at IS NULL
FOR UPDATE;

-- name: UpdateAPIKeyHash :exec
UPDATE api_keys SET key_hash = @key_hash
WHERE project_id = @project_id AND id = @id;

-- Lock active admin keys in a stable order before counting them, so two
-- concurrent revocations cannot remove the last admin key.
-- name: LockActiveAdminAPIKeyIDs :many
SELECT id FROM api_keys
WHERE project_id = @project_id AND revoked_at IS NULL AND 'admin' = ANY(scopes)
ORDER BY id
FOR UPDATE;

-- name: RevokeAPIKey :one
UPDATE api_keys SET revoked_at = @revoked_at::timestamptz
WHERE project_id = @project_id AND id = @id AND revoked_at IS NULL
RETURNING id;

-- name: InsertAPIKeyEvent :exec
INSERT INTO api_key_events (id, project_id, api_key_id, actor_key_id, action, created_at)
VALUES (@id, @project_id, @api_key_id, @actor_key_id, @action, @created_at);

-- A restart with the same bootstrap key leaves the row untouched, so a revoked
-- or narrowed bootstrap key stays that way; a different key rotates and
-- reactivates it, which keeps lost-admin recovery possible.
-- name: EnsureBootstrapAPIKey :exec
INSERT INTO api_keys (id, project_id, name, key_hash, scopes)
VALUES (@id, @project_id, 'bootstrap', @key_hash, ARRAY['read', 'write', 'ingest', 'replay', 'admin'])
ON CONFLICT (project_id, name) DO UPDATE
SET key_hash = EXCLUDED.key_hash, scopes = EXCLUDED.scopes, revoked_at = NULL
WHERE api_keys.key_hash <> EXCLUDED.key_hash;

-- name: AuthenticateAPIKey :one
SELECT id, project_id, scopes
FROM api_keys
WHERE key_hash = @key_hash
  AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > now());
