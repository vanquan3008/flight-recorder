-- Restores the API key tables empty; accounts, sessions, ingest tokens and the audit trail are dropped.
CREATE TABLE api_keys (
    id           uuid PRIMARY KEY,
    project_id   uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name         text NOT NULL,
    key_hash     bytea NOT NULL UNIQUE,
    scopes       text[] NOT NULL DEFAULT '{}',
    expires_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    revoked_at   timestamptz,
    UNIQUE (project_id, name),
    CONSTRAINT api_keys_project_id_id_unique UNIQUE (project_id, id)
);

CREATE TABLE api_key_events (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    api_key_id uuid NOT NULL,
    actor_key_id uuid NOT NULL,
    action text NOT NULL CHECK (action IN ('CREATED', 'ROTATED', 'REVOKED')),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (project_id, api_key_id) REFERENCES api_keys(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, actor_key_id) REFERENCES api_keys(project_id, id) ON DELETE RESTRICT
);

ALTER TABLE incident_status_changes DROP COLUMN actor_user_id;
ALTER TABLE incident_status_changes ADD COLUMN actor_key_id uuid;
ALTER TABLE incident_status_changes ADD FOREIGN KEY (project_id, actor_key_id) REFERENCES api_keys(project_id, id);

DROP TABLE audit_events;
DROP TABLE ingest_tokens;
DROP TABLE sessions;
DROP TABLE project_members;
DROP TABLE users;
