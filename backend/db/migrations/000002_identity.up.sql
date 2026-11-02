-- Identity and access. People register an account and sign in with a short-lived JWT; telemetry senders
-- use per-project ingest tokens. API keys are gone.

CREATE TABLE users (
    id                    uuid PRIMARY KEY,
    email                 text NOT NULL,
    display_name          text NOT NULL,
    password_hash         text NOT NULL,
    platform_admin        boolean NOT NULL DEFAULT false,
    must_change_password  boolean NOT NULL DEFAULT false,
    disabled_at           timestamptz,
    -- Access tokens issued before this moment are refused: a password change or a disabled
    -- account ends them at once instead of when they expire.
    tokens_valid_after    timestamptz NOT NULL DEFAULT to_timestamp(0),
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    last_login_at         timestamptz,
    CONSTRAINT users_email_format CHECK (email = lower(email) AND length(email) <= 254 AND email ~ '^[^@[:space:]]+@[^@[:space:]]+$'),
    CONSTRAINT users_display_name_length CHECK (length(btrim(display_name)) BETWEEN 1 AND 100)
);
CREATE UNIQUE INDEX users_email_key ON users (email);

-- A project has owners (who made it, and run it) and users (who were allowed to see its telemetry).
CREATE TABLE project_members (
    project_id  uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, user_id),
    CONSTRAINT project_members_role CHECK (role IN ('owner', 'user'))
);
CREATE INDEX project_members_user_idx ON project_members (user_id);

-- Refresh sessions. Only a digest of the opaque refresh token is stored, and each use replaces it.
CREATE TABLE sessions (
    token_hash      bytea PRIMARY KEY,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at      timestamptz NOT NULL DEFAULT now(),
    last_seen_at    timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    max_expires_at  timestamptz NOT NULL,
    user_agent      text NOT NULL DEFAULT '',
    CONSTRAINT sessions_expiry_order CHECK (expires_at <= max_expires_at)
);
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (max_expires_at);

-- Credentials for telemetry senders. The token itself is a signed JWT shown once; this row is what
-- lets an owner list and revoke it.
CREATE TABLE ingest_tokens (
    id          uuid PRIMARY KEY,
    project_id  uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        text NOT NULL,
    created_by  uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    revoked_at  timestamptz,
    UNIQUE (project_id, name),
    CONSTRAINT ingest_tokens_name_format CHECK (name ~ '^[A-Za-z0-9][A-Za-z0-9 ._-]{0,63}$')
);

-- Who did what in the control plane: accounts, projects, membership and ingest tokens.
CREATE TABLE audit_events (
    id             uuid PRIMARY KEY,
    project_id     uuid REFERENCES projects(id) ON DELETE CASCADE,
    actor_user_id  uuid REFERENCES users(id) ON DELETE SET NULL,
    action         text NOT NULL,
    target_type    text NOT NULL,
    target_id      text NOT NULL,
    detail         jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT audit_events_detail_object CHECK (jsonb_typeof(detail) = 'object')
);
CREATE INDEX audit_events_project_idx ON audit_events (project_id, created_at DESC, id DESC);
CREATE INDEX audit_events_created_idx ON audit_events (created_at);

-- API keys are replaced by accounts and ingest tokens. Incident history names the person who acted.
DROP TABLE api_key_events;
ALTER TABLE incident_status_changes DROP COLUMN actor_key_id;
ALTER TABLE incident_status_changes ADD COLUMN actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL;
DROP TABLE api_keys;
