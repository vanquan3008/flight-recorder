CREATE TABLE spans (
    event_id                 uuid PRIMARY KEY,
    project_id               uuid NOT NULL,
    environment_id           uuid NOT NULL,
    trace_id                 text NOT NULL,
    span_id                  text NOT NULL,
    parent_span_id           text,
    trace_state              text NOT NULL DEFAULT '',
    span_name                text NOT NULL,
    span_kind                integer NOT NULL,
    started_at               timestamptz NOT NULL,
    ended_at                 timestamptz NOT NULL,
    status_code              text NOT NULL,
    status_message           text NOT NULL DEFAULT '',
    flags                    bigint NOT NULL DEFAULT 0,
    service_name             text NOT NULL,
    service_version          text,
    resource_schema_url      text NOT NULL DEFAULT '',
    resource_attributes      jsonb NOT NULL DEFAULT '{}'::jsonb,
    scope_name               text NOT NULL DEFAULT '',
    scope_version            text NOT NULL DEFAULT '',
    scope_schema_url         text NOT NULL DEFAULT '',
    scope_attributes         jsonb NOT NULL DEFAULT '{}'::jsonb,
    span_attributes          jsonb NOT NULL DEFAULT '{}'::jsonb,
    span_events              jsonb NOT NULL DEFAULT '[]'::jsonb,
    span_links               jsonb NOT NULL DEFAULT '[]'::jsonb,
    dropped_attributes_count integer NOT NULL DEFAULT 0,
    dropped_events_count     integer NOT NULL DEFAULT 0,
    dropped_links_count      integer NOT NULL DEFAULT 0,
    FOREIGN KEY (project_id, event_id) REFERENCES events(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id),
    UNIQUE (project_id, trace_id, span_id),
    CONSTRAINT spans_resource_attributes_object CHECK (jsonb_typeof(resource_attributes) = 'object'),
    CONSTRAINT spans_scope_attributes_object CHECK (jsonb_typeof(scope_attributes) = 'object'),
    CONSTRAINT spans_span_attributes_object CHECK (jsonb_typeof(span_attributes) = 'object'),
    CONSTRAINT spans_events_array CHECK (jsonb_typeof(span_events) = 'array'),
    CONSTRAINT spans_links_array CHECK (jsonb_typeof(span_links) = 'array')
);

CREATE TABLE logs (
    event_id                 uuid PRIMARY KEY,
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
    FOREIGN KEY (project_id, event_id) REFERENCES events(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id),
    CONSTRAINT logs_resource_attributes_object CHECK (jsonb_typeof(resource_attributes) = 'object'),
    CONSTRAINT logs_scope_attributes_object CHECK (jsonb_typeof(scope_attributes) = 'object'),
    CONSTRAINT logs_attributes_object CHECK (jsonb_typeof(log_attributes) = 'object')
);

CREATE INDEX spans_trace_time_idx
    ON spans (project_id, trace_id, started_at, event_id);
CREATE INDEX spans_service_time_idx
    ON spans (project_id, environment_id, service_name, started_at DESC, event_id DESC);
CREATE INDEX logs_time_idx
    ON logs (project_id, environment_id, occurred_at DESC, event_id DESC);
CREATE INDEX logs_trace_idx
    ON logs (project_id, trace_id, occurred_at, event_id) WHERE trace_id IS NOT NULL;
