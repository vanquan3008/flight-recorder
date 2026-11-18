-- Accounts

-- name: InsertUser :one
INSERT INTO users (id, email, display_name, password_hash, platform_admin, must_change_password)
VALUES (@id, @email, @display_name, @password_hash, @platform_admin, @must_change_password)
RETURNING *;

-- name: UsersExist :one
SELECT EXISTS (SELECT 1 FROM users);

-- name: GetUser :one
SELECT * FROM users WHERE id = @id;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = @email;

-- name: LockUser :one
SELECT * FROM users WHERE id = @id FOR UPDATE;

-- name: CountOtherActivePlatformAdmins :one
SELECT count(*) FROM users
WHERE platform_admin AND disabled_at IS NULL AND id <> @id;

-- Revocation markers advance even for concurrent changes or a clock moving backwards.
-- name: UpdateUser :one
UPDATE users SET
    display_name   = COALESCE(sqlc.narg(display_name), display_name),
    platform_admin = COALESCE(sqlc.narg(platform_admin)::boolean, platform_admin),
    disabled_at    = CASE
        WHEN sqlc.narg(disabled)::boolean IS NULL THEN disabled_at
        WHEN sqlc.narg(disabled)::boolean THEN COALESCE(disabled_at, now())
        ELSE NULL END,
    tokens_valid_after = CASE WHEN sqlc.narg(disabled)::boolean
        THEN greatest(tokens_valid_after + interval '1 microsecond', sqlc.arg(valid_after)::timestamptz)
        ELSE tokens_valid_after END,
    updated_at     = now()
WHERE id = @id
RETURNING *;

-- name: SetUserPassword :execrows
UPDATE users
SET password_hash = @password_hash, must_change_password = @must_change_password,
    tokens_valid_after = greatest(tokens_valid_after + interval '1 microsecond', sqlc.arg(valid_after)::timestamptz), updated_at = now()
WHERE id = @id;

-- name: RecordUserLogin :exec
UPDATE users SET last_login_at = now() WHERE id = @id;

-- name: ListUsers :many
SELECT * FROM users
WHERE (sqlc.narg(before)::timestamptz IS NULL
       OR created_at < sqlc.narg(before)
       OR (created_at = sqlc.narg(before) AND id < sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer;

-- Sessions: only a digest of the token is stored.

-- name: InsertSession :exec
INSERT INTO sessions (token_hash, user_id, created_at, last_seen_at, expires_at, max_expires_at, user_agent)
VALUES (@token_hash, @user_id, @created_at, @created_at, @expires_at, @max_expires_at, @user_agent);

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE max_expires_at < now() OR expires_at < now();

-- A session of a disabled account, or past its idle or absolute lifetime, is not returned.
-- name: GetSessionUser :one
SELECT s.expires_at, s.max_expires_at, sqlc.embed(u)
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = @token_hash
  AND s.expires_at > now()
  AND s.max_expires_at > now()
  AND u.disabled_at IS NULL;

-- Each refresh replaces the token: a used token stops working, so a stolen copy is good for one use at most.
-- name: RotateSession :execrows
UPDATE sessions
SET token_hash = @new_token_hash, last_seen_at = @last_seen_at, expires_at = @expires_at
WHERE token_hash = @old_token_hash AND expires_at > now() AND max_expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = @token_hash;

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = @user_id;

-- name: DeleteUserSessionsExcept :exec
DELETE FROM sessions WHERE user_id = @user_id AND token_hash <> @keep_token_hash;

-- Projects and environments

-- name: InsertProject :one
INSERT INTO projects (id, slug, name) VALUES (@id, @slug, @name)
RETURNING *;

-- name: InsertEnvironment :one
INSERT INTO environments (id, project_id, name) VALUES (@id, @project_id, @name)
RETURNING *;

-- name: GetProjectByReference :one
SELECT * FROM projects WHERE id::text = @reference OR slug = @reference;

-- name: RenameProject :one
UPDATE projects SET name = @name, updated_at = now()
WHERE id = @id
RETURNING *;

-- name: GetProjectRetention :one
SELECT telemetry_retention_days FROM projects WHERE id = @project_id;

-- name: UpdateProjectRetention :one
UPDATE projects SET telemetry_retention_days = sqlc.narg(retention_days), updated_at = now()
WHERE id = @project_id
RETURNING telemetry_retention_days;

-- name: ListEnvironments :many
SELECT * FROM environments WHERE project_id = @project_id ORDER BY created_at, id;

-- name: ListUserProjects :many
SELECT sqlc.embed(p), m.role
FROM project_members m
JOIN projects p ON p.id = m.project_id
WHERE m.user_id = @user_id
ORDER BY p.name, p.id;

-- name: ListAllProjects :many
SELECT * FROM projects ORDER BY name, id;

-- Membership

-- name: GetMemberRole :one
SELECT role FROM project_members WHERE project_id = @project_id AND user_id = @user_id;

-- name: LockMemberRole :one
SELECT role FROM project_members
WHERE project_id = @project_id AND user_id = @user_id
FOR UPDATE;

-- name: ListProjectMembers :many
SELECT sqlc.embed(u), m.role, m.created_at AS member_since
FROM project_members m
JOIN users u ON u.id = m.user_id
WHERE m.project_id = @project_id
ORDER BY m.created_at, u.id;

-- name: InsertProjectMember :exec
INSERT INTO project_members (project_id, user_id, role) VALUES (@project_id, @user_id, @role);

-- name: UpdateProjectMemberRole :exec
UPDATE project_members SET role = @role, updated_at = now()
WHERE project_id = @project_id AND user_id = @user_id;

-- name: DeleteProjectMember :exec
DELETE FROM project_members WHERE project_id = @project_id AND user_id = @user_id;

-- A disabled owner cannot act, so it does not count as keeping the project owned.
-- name: CountOtherActiveOwners :one
SELECT count(*)
FROM project_members m
JOIN users u ON u.id = m.user_id
WHERE m.project_id = @project_id AND m.role = 'owner' AND m.user_id <> @user_id AND u.disabled_at IS NULL;

-- Audit trail of control-plane changes

-- name: InsertAuditEvent :exec
INSERT INTO audit_events (id, project_id, actor_user_id, action, target_type, target_id, detail)
VALUES (@id, sqlc.narg(project_id), sqlc.narg(actor_user_id), @action, @target_type, @target_id, @detail);

-- name: ListAuditEvents :many
SELECT id, project_id, actor_user_id, action, target_type, target_id, detail, created_at
FROM audit_events
WHERE project_id = @project_id
  AND (sqlc.narg(before)::timestamptz IS NULL
       OR created_at < sqlc.narg(before)
       OR (created_at = sqlc.narg(before) AND id < sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer;

-- Every project and environment, for work that runs for all of them, such as incident detection.
-- name: ListScopes :many
SELECT sqlc.embed(p), sqlc.embed(e)
FROM environments e
JOIN projects p ON p.id = e.project_id
ORDER BY p.id, e.created_at, e.id;

-- Projects a person created and runs; the console limits how many an ordinary account can open.
-- name: CountOwnedProjects :one
SELECT count(*) FROM project_members WHERE user_id = @user_id AND role = 'owner';

-- Ingest tokens: credentials for telemetry senders, one JWT per row.

-- name: InsertIngestToken :one
INSERT INTO ingest_tokens (id, project_id, name, created_by, expires_at)
VALUES (@id, @project_id, @name, sqlc.narg(created_by), @expires_at)
RETURNING *;

-- name: GetIngestToken :one
SELECT * FROM ingest_tokens WHERE id = @id;

-- name: ListIngestTokens :many
SELECT * FROM ingest_tokens
WHERE project_id = @project_id
  AND (sqlc.narg(before)::timestamptz IS NULL
       OR created_at < sqlc.narg(before)
       OR (created_at = sqlc.narg(before) AND id < sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer;

-- name: RevokeIngestToken :one
UPDATE ingest_tokens SET revoked_at = now()
WHERE project_id = @project_id AND id = @id AND revoked_at IS NULL
RETURNING id;
