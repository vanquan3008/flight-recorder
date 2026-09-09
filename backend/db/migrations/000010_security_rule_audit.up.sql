CREATE TABLE security_rule_audits (
    id              uuid PRIMARY KEY,
    project_id      uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    rule_id         uuid NOT NULL,
    rule_version    integer NOT NULL CHECK (rule_version > 0),
    action          text NOT NULL CHECK (action IN ('CREATED', 'UPDATED', 'DISABLED')),
    actor_key_id    uuid,
    rule_snapshot   jsonb NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (actor_key_id) REFERENCES api_keys(id) ON DELETE SET NULL (actor_key_id),
    CONSTRAINT security_rule_audits_snapshot_object CHECK (jsonb_typeof(rule_snapshot) = 'object'),
    UNIQUE (project_id, rule_id, rule_version, action)
);

CREATE INDEX security_rule_audits_rule_idx
    ON security_rule_audits (project_id, rule_id, created_at DESC, id DESC);
