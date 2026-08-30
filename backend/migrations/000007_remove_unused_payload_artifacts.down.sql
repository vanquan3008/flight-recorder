CREATE TABLE payload_artifacts (
    id            uuid PRIMARY KEY,
    project_id    uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    storage_key   text NOT NULL,
    content_type  text NOT NULL,
    size_bytes    bigint NOT NULL CHECK (size_bytes >= 0),
    sha256        bytea NOT NULL,
    truncated     boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, storage_key),
    UNIQUE (project_id, id)
);

ALTER TABLE events
    ADD COLUMN payload_ref uuid,
    ADD CONSTRAINT events_project_id_payload_ref_fkey
        FOREIGN KEY (project_id, payload_ref) REFERENCES payload_artifacts(project_id, id);

ALTER TABLE replay_executions
    ADD COLUMN result_payload_ref uuid,
    ADD CONSTRAINT replay_executions_project_id_result_payload_ref_fkey
        FOREIGN KEY (project_id, result_payload_ref) REFERENCES payload_artifacts(project_id, id);
