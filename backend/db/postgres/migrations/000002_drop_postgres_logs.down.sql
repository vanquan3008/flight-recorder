-- Restores the empty logs table the previous release expects. Dropped rows are not recovered.
BEGIN;

CREATE TABLE logs (
    event_id                 uuid NOT NULL,
    project_id               uuid NOT NULL,
    environment_id           uuid NOT NULL,
    occurred_at              timestamptz NOT NULL,
    observed_at              timestamptz,
    trace_id                 text,
    span_id                  text,
    trace_flags              bigint NOT NULL DEFAULT 0,
    severity_number          integer NOT NULL DEFAULT 0,
    severity_text            text NOT NULL DEFAULT '',
    body                     jsonb,
    event_name               text NOT NULL DEFAULT '',
    service_name             text NOT NULL,
    service_version          text,
    resource_schema_url      text NOT NULL DEFAULT '',
    resource_attributes      jsonb NOT NULL DEFAULT '{}'::jsonb,
    scope_name               text NOT NULL DEFAULT '',
    scope_version            text NOT NULL DEFAULT '',
    scope_schema_url         text NOT NULL DEFAULT '',
    scope_attributes         jsonb NOT NULL DEFAULT '{}'::jsonb,
    log_attributes           jsonb NOT NULL DEFAULT '{}'::jsonb,
    dropped_attributes_count integer NOT NULL DEFAULT 0,
    PRIMARY KEY (event_id, occurred_at),
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id),
    CONSTRAINT logs_resource_attributes_object CHECK (jsonb_typeof(resource_attributes) = 'object'),
    CONSTRAINT logs_scope_attributes_object CHECK (jsonb_typeof(scope_attributes) = 'object'),
    CONSTRAINT logs_attributes_object CHECK (jsonb_typeof(log_attributes) = 'object')
) PARTITION BY RANGE (occurred_at);

CREATE TABLE logs_default PARTITION OF logs DEFAULT;

CREATE INDEX logs_time_idx
    ON logs (project_id, environment_id, occurred_at DESC, event_id DESC);

CREATE INDEX logs_trace_idx
    ON logs (project_id, trace_id, occurred_at, event_id) WHERE trace_id IS NOT NULL;

CREATE INDEX logs_body_trgm_idx ON logs USING GIN (
    (body::text) gin_trgm_ops
) WHERE body IS NOT NULL;

COMMIT;
