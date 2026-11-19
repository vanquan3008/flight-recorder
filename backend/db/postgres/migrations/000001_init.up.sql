-- Initial PostgreSQL baseline. Later schema changes need a new migration version.
BEGIN;

-- identity
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

CREATE TABLE users (
    id                    uuid PRIMARY KEY,
    email                 text NOT NULL,
    display_name          text NOT NULL,
    password_hash         text NOT NULL,
    platform_admin        boolean NOT NULL DEFAULT false,
    must_change_password  boolean NOT NULL DEFAULT false,
    disabled_at           timestamptz,
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

-- incident envelopes
-- Failing spans and logs keep a compact envelope here for incident timelines; telemetry lives in ClickHouse.

CREATE TABLE events (
    id               uuid NOT NULL,
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
    ingested_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, occurred_at),
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id),
    CONSTRAINT events_attributes_object CHECK (jsonb_typeof(attributes) = 'object')
) PARTITION BY RANGE (occurred_at);

CREATE TABLE events_default PARTITION OF events DEFAULT;

-- incidents
-- Incidents, investigation history, evidence and replay executions.

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
    CONSTRAINT incidents_time_order CHECK (last_seen_at >= first_seen_at),
    CONSTRAINT incidents_status_check CHECK (status IN ('OPEN', 'INVESTIGATING', 'RESOLVED', 'IGNORED')),
    CONSTRAINT incidents_severity_check CHECK (severity IN ('ERROR', 'CRITICAL'))
);

CREATE INDEX incidents_browse_idx
    ON incidents (project_id, environment_id, last_seen_at DESC, id DESC);

CREATE INDEX incidents_status_idx
    ON incidents (project_id, status, severity, last_seen_at DESC);

CREATE INDEX incidents_fingerprint_idx
    ON incidents (project_id, environment_id, fingerprint, last_seen_at DESC);

-- incident_occurrences and incident_events reference events by id without a
-- foreign key. Retention drops old event partitions, so these rows may outlive
-- their event; readers must join with an inner join and tolerate the gap.
CREATE TABLE incident_occurrences (
    id           uuid PRIMARY KEY,
    project_id   uuid NOT NULL,
    incident_id  uuid NOT NULL,
    event_id     uuid NOT NULL,
    occurred_at  timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, incident_id) REFERENCES incidents(project_id, id) ON DELETE CASCADE,
    UNIQUE (incident_id, event_id)
);

-- events has no secondary indexes on purpose: it is only joined by
-- (project_id, id) for incident evidence and counted by time for retention.
CREATE INDEX incident_occurrences_time_idx
    ON incident_occurrences (incident_id, occurred_at DESC, id DESC);

CREATE UNIQUE INDEX incident_occurrences_project_event_idx
    ON incident_occurrences (project_id, event_id);

CREATE TABLE incident_events (
    project_id   uuid NOT NULL,
    incident_id  uuid NOT NULL,
    event_id     uuid NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (incident_id, event_id),
    FOREIGN KEY (project_id, incident_id) REFERENCES incidents(project_id, id) ON DELETE CASCADE
);

CREATE TABLE incident_status_changes (
    id              uuid PRIMARY KEY,
    project_id      uuid NOT NULL,
    incident_id     uuid NOT NULL,
    previous_status text NOT NULL,
    status          text NOT NULL,
    changed_at      timestamptz NOT NULL,
    actor_user_id   uuid REFERENCES users(id) ON DELETE SET NULL,
    FOREIGN KEY (project_id, incident_id) REFERENCES incidents(project_id, id) ON DELETE CASCADE,
    CONSTRAINT incident_status_changes_status CHECK (status IN ('OPEN', 'INVESTIGATING', 'RESOLVED', 'IGNORED')),
    CONSTRAINT incident_status_changes_previous_status CHECK (previous_status IN ('OPEN', 'INVESTIGATING', 'RESOLVED', 'IGNORED'))
);

CREATE INDEX incident_status_changes_timeline_idx
    ON incident_status_changes (project_id, incident_id, changed_at DESC, id DESC);

CREATE TABLE incident_evidence (
    id                  uuid PRIMARY KEY,
    project_id          uuid NOT NULL,
    environment_id      uuid NOT NULL,
    incident_id         uuid NOT NULL,
    signal_type         text NOT NULL,
    entity_type         text NOT NULL,
    entity_id           text NOT NULL,
    metric_name         text NOT NULL,
    direction           text NOT NULL CHECK (direction IN ('increase', 'decrease', 'event')),
    observed_value      double precision NOT NULL,
    baseline_value      double precision NOT NULL,
    change_ratio        double precision NOT NULL,
    window_started_at   timestamptz NOT NULL,
    window_ended_at     timestamptz NOT NULL,
    trace_ids           text[] NOT NULL DEFAULT '{}',
    summary             text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    series_id           text NOT NULL DEFAULT '',
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, incident_id) REFERENCES incidents(project_id, id) ON DELETE CASCADE,
    CONSTRAINT incident_evidence_signal_series_unique
        UNIQUE (project_id, incident_id, signal_type, entity_id, metric_name, series_id, window_ended_at),
    CONSTRAINT incident_evidence_window_order CHECK (window_ended_at >= window_started_at)
);

CREATE INDEX incident_evidence_timeline_idx
    ON incident_evidence (project_id, environment_id, incident_id, window_ended_at DESC, id DESC);

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
    comparison          jsonb,
    created_by          text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, incident_id) REFERENCES incidents(project_id, id) ON DELETE CASCADE
);

CREATE INDEX replay_executions_incident_idx
    ON replay_executions (incident_id, created_at DESC, id DESC);

-- notifications
-- Webhook configuration and durable delivery outbox.

-- Webhook notification delivery: operators register HTTPS endpoints and select
-- which event types trigger a delivery attempt. Deliveries are retried up to
-- max_attempts times by the durable notification worker.
CREATE TABLE webhooks (
    id           uuid PRIMARY KEY,
    project_id   uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name         text NOT NULL,
    url          text NOT NULL,
    secret       text NOT NULL,          -- HMAC-SHA256 signing secret (protected database value)
    events       text[] NOT NULL,        -- e.g. ['incident.opened']
    enabled      bool NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, name),
    UNIQUE (project_id, id),
    CONSTRAINT webhooks_url_format   CHECK (url   LIKE 'https://%'),
    CONSTRAINT webhooks_events_nonempty CHECK (cardinality(events) > 0),
    CONSTRAINT webhooks_events_supported CHECK (
        array_ndims(events) = 1
        AND events <@ ARRAY['incident.opened', 'incident.resolved']::text[]
    )
);

CREATE INDEX webhooks_project_idx ON webhooks (project_id, enabled);

CREATE TABLE webhook_deliveries (
    id             uuid PRIMARY KEY,
    project_id     uuid NOT NULL,
    webhook_id     uuid NOT NULL,
    event_type     text NOT NULL,
    payload        jsonb NOT NULL,
    status         text NOT NULL DEFAULT 'pending',  -- pending, sending, succeeded, failed
    attempt        integer NOT NULL DEFAULT 0,
    max_attempts   integer NOT NULL DEFAULT 3,
    http_status    integer,
    last_error     text,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, webhook_id) REFERENCES webhooks(project_id, id) ON DELETE CASCADE,
    UNIQUE (project_id, id),
    CONSTRAINT webhook_delivery_status CHECK (status IN ('pending', 'sending', 'succeeded', 'failed'))
);

CREATE INDEX webhook_deliveries_pending_idx
    ON webhook_deliveries (project_id, next_attempt_at)
    WHERE status = 'pending';

CREATE INDEX webhook_deliveries_webhook_idx
    ON webhook_deliveries (project_id, webhook_id, created_at DESC, id DESC);

COMMIT;
