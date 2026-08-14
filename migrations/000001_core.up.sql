CREATE TABLE projects (
    id          uuid PRIMARY KEY,
    slug        text NOT NULL UNIQUE,
    name        text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT projects_slug_format CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,62}$')
);

CREATE TABLE environments (
    id          uuid PRIMARY KEY,
    project_id  uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, name),
    UNIQUE (project_id, id)
);

CREATE TABLE services (
    id              uuid PRIMARY KEY,
    project_id      uuid NOT NULL,
    environment_id  uuid NOT NULL,
    name            text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id) ON DELETE CASCADE,
    UNIQUE (project_id, environment_id, name),
    UNIQUE (project_id, id)
);

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

CREATE TABLE events (
    id               uuid PRIMARY KEY,
    project_id       uuid NOT NULL,
    environment_id   uuid NOT NULL,
    service_name     text NOT NULL,
    service_version  text,
    trace_id         text,
    span_id          text,
    parent_span_id   text,
    request_id       text,
    event_type       text NOT NULL,
    occurred_at      timestamptz NOT NULL,
    duration_ns      bigint CHECK (duration_ns IS NULL OR duration_ns >= 0),
    severity         text,
    status           text,
    http_status      integer CHECK (http_status IS NULL OR http_status BETWEEN 100 AND 599),
    operation        text,
    error_type       text,
    error_message    text,
    attributes       jsonb NOT NULL DEFAULT '{}'::jsonb,
    payload_ref      uuid,
    ingested_at      timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id),
    FOREIGN KEY (project_id, payload_ref) REFERENCES payload_artifacts(project_id, id),
    UNIQUE (project_id, id),
    CONSTRAINT events_attributes_object CHECK (jsonb_typeof(attributes) = 'object')
);

CREATE TABLE incidents (
    id                uuid PRIMARY KEY,
    project_id        uuid NOT NULL,
    environment_id    uuid NOT NULL,
    primary_service   text NOT NULL,
    trace_id          text,
    request_id        text,
    fingerprint       text NOT NULL,
    title             text NOT NULL,
    error_type        text,
    error_message     text,
    http_status       integer CHECK (http_status IS NULL OR http_status BETWEEN 100 AND 599),
    severity          text NOT NULL,
    status            text NOT NULL,
    first_seen_at     timestamptz NOT NULL,
    last_seen_at      timestamptz NOT NULL,
    occurrence_count  bigint NOT NULL DEFAULT 1 CHECK (occurrence_count > 0),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id),
    UNIQUE (project_id, id),
    CONSTRAINT incidents_time_order CHECK (last_seen_at >= first_seen_at)
);

CREATE TABLE incident_occurrences (
    id           uuid PRIMARY KEY,
    project_id   uuid NOT NULL,
    incident_id  uuid NOT NULL,
    event_id     uuid NOT NULL,
    occurred_at  timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, incident_id) REFERENCES incidents(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, event_id) REFERENCES events(project_id, id) ON DELETE CASCADE,
    UNIQUE (incident_id, event_id)
);

CREATE TABLE incident_events (
    project_id   uuid NOT NULL,
    incident_id  uuid NOT NULL,
    event_id     uuid NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (incident_id, event_id),
    FOREIGN KEY (project_id, incident_id) REFERENCES incidents(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, event_id) REFERENCES events(project_id, id) ON DELETE CASCADE
);

CREATE TABLE api_keys (
    id           uuid PRIMARY KEY,
    project_id   uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name         text NOT NULL,
    key_hash     bytea NOT NULL UNIQUE,
    scopes       text[] NOT NULL DEFAULT '{}',
    last_used_at timestamptz,
    expires_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    revoked_at   timestamptz,
    UNIQUE (project_id, name)
);

CREATE TABLE replay_executions (
    id                  uuid PRIMARY KEY,
    project_id          uuid NOT NULL,
    incident_id         uuid NOT NULL,
    target              text NOT NULL,
    status              text NOT NULL,
    started_at          timestamptz,
    completed_at        timestamptz,
    duration_ns         bigint CHECK (duration_ns IS NULL OR duration_ns >= 0),
    http_status         integer CHECK (http_status IS NULL OR http_status BETWEEN 100 AND 599),
    result_payload_ref  uuid,
    comparison          jsonb,
    created_by          text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, incident_id) REFERENCES incidents(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, result_payload_ref) REFERENCES payload_artifacts(project_id, id)
);

CREATE INDEX incidents_browse_idx
    ON incidents (project_id, environment_id, last_seen_at DESC, id DESC);
CREATE INDEX incidents_status_idx
    ON incidents (project_id, status, severity, last_seen_at DESC);
CREATE INDEX incidents_fingerprint_idx
    ON incidents (project_id, environment_id, fingerprint, last_seen_at DESC);
CREATE INDEX events_trace_idx
    ON events (project_id, trace_id, occurred_at, id) WHERE trace_id IS NOT NULL;
CREATE INDEX events_request_idx
    ON events (project_id, request_id, occurred_at, id) WHERE request_id IS NOT NULL;
CREATE INDEX events_service_time_idx
    ON events (project_id, environment_id, service_name, occurred_at DESC, id DESC);
CREATE INDEX events_log_search_idx
    ON events (project_id, environment_id, service_name, severity, occurred_at DESC)
    WHERE event_type = 'LOG';
CREATE INDEX incident_occurrences_time_idx
    ON incident_occurrences (incident_id, occurred_at DESC, id DESC);
CREATE INDEX replay_executions_incident_idx
    ON replay_executions (incident_id, created_at DESC, id DESC);
