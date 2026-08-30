ALTER TABLE api_keys
    ADD CONSTRAINT api_keys_project_id_id_unique UNIQUE (project_id, id);

CREATE TABLE incident_status_changes (
    id              uuid PRIMARY KEY,
    project_id      uuid NOT NULL,
    incident_id     uuid NOT NULL,
    previous_status text NOT NULL,
    status          text NOT NULL,
    actor_key_id    uuid NOT NULL,
    changed_at      timestamptz NOT NULL,
    FOREIGN KEY (project_id, incident_id) REFERENCES incidents(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, actor_key_id) REFERENCES api_keys(project_id, id),
    CONSTRAINT incident_status_changes_status CHECK (status IN ('OPEN', 'INVESTIGATING', 'RESOLVED', 'IGNORED')),
    CONSTRAINT incident_status_changes_previous_status CHECK (previous_status IN ('OPEN', 'INVESTIGATING', 'RESOLVED', 'IGNORED'))
);

CREATE INDEX incident_status_changes_timeline_idx
    ON incident_status_changes (project_id, incident_id, changed_at DESC, id DESC);
