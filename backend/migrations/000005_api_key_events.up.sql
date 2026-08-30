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

CREATE INDEX api_key_events_project_key_created_idx
    ON api_key_events (project_id, api_key_id, created_at DESC, id DESC);
