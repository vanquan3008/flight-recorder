CREATE TABLE security_sources (
    id                   uuid PRIMARY KEY,
    project_id           uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    environment_id       uuid NOT NULL,
    name                 text NOT NULL,
    source_type          text NOT NULL,
    format               text NOT NULL,
    last_seen_at         timestamptz NOT NULL,
    accepted_count       bigint NOT NULL DEFAULT 0 CHECK (accepted_count >= 0),
    error_count          bigint NOT NULL DEFAULT 0 CHECK (error_count >= 0),
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id) ON DELETE CASCADE,
    UNIQUE (project_id, environment_id, name),
    UNIQUE (project_id, id)
);

CREATE TABLE security_events (
    id                   uuid PRIMARY KEY,
    project_id           uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    environment_id       uuid NOT NULL,
    raw_event_id         uuid NOT NULL,
    source_id            uuid,
    occurred_at          timestamptz NOT NULL,
    ingested_at          timestamptz NOT NULL DEFAULT now(),
    format               text NOT NULL,
    event_kind           text NOT NULL,
    category             text,
    action               text,
    outcome              text,
    severity             text,
    observer_vendor      text,
    observer_product     text,
    observer_hostname    text,
    host_name            text,
    user_name            text,
    source_ip            inet,
    source_port          integer CHECK (source_port BETWEEN 0 AND 65535),
    destination_ip       inet,
    destination_port     integer CHECK (destination_port BETWEEN 0 AND 65535),
    network_transport    text,
    network_protocol     text,
    rule_identifier      text,
    rule_name            text,
    message              text,
    attributes           jsonb NOT NULL DEFAULT '{}'::jsonb,
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, raw_event_id) REFERENCES events(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, source_id) REFERENCES security_sources(project_id, id) ON DELETE SET NULL (source_id),
    UNIQUE (project_id, id),
    UNIQUE (project_id, raw_event_id),
    CONSTRAINT security_events_attributes_object CHECK (jsonb_typeof(attributes) = 'object')
);

CREATE TABLE security_detection_rules (
    id                 uuid PRIMARY KEY,
    project_id         uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name               text NOT NULL,
    description        text,
    version            integer NOT NULL DEFAULT 1 CHECK (version > 0),
    enabled            boolean NOT NULL DEFAULT true,
    severity           text NOT NULL,
    match_field        text NOT NULL,
    match_operator     text NOT NULL,
    match_value        text NOT NULL DEFAULT '',
    threshold_count    integer NOT NULL DEFAULT 1 CHECK (threshold_count > 0 AND threshold_count <= 10000),
    group_by_field     text,
    window_seconds     integer NOT NULL CHECK (window_seconds > 0 AND window_seconds <= 604800),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, name),
    UNIQUE (project_id, id)
);

CREATE TABLE security_detections (
    id                   uuid PRIMARY KEY,
    project_id           uuid NOT NULL,
    environment_id       uuid NOT NULL,
    rule_id              uuid NOT NULL,
    rule_version         integer NOT NULL,
    security_event_id    uuid NOT NULL,
    incident_id          uuid,
    group_key            text NOT NULL DEFAULT '',
    window_started_at    timestamptz NOT NULL,
    window_ended_at      timestamptz NOT NULL,
    severity             text NOT NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, rule_id) REFERENCES security_detection_rules(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, security_event_id) REFERENCES security_events(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, incident_id) REFERENCES incidents(project_id, id) ON DELETE SET NULL (incident_id),
    UNIQUE (project_id, rule_id, security_event_id),
    UNIQUE (project_id, id)
);

CREATE TABLE security_detection_checkpoints (
    project_id          uuid NOT NULL,
    environment_id      uuid NOT NULL,
    last_ingested_at    timestamptz NOT NULL,
    last_event_id       uuid NOT NULL,
    updated_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, environment_id),
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id) ON DELETE CASCADE
);

CREATE INDEX security_events_browse_idx ON security_events (project_id, environment_id, occurred_at DESC, id DESC);
CREATE INDEX security_events_source_idx ON security_events (project_id, environment_id, source_ip, occurred_at DESC) WHERE source_ip IS NOT NULL;
CREATE INDEX security_events_destination_idx ON security_events (project_id, environment_id, destination_ip, occurred_at DESC) WHERE destination_ip IS NOT NULL;
CREATE INDEX security_events_ingest_idx ON security_events (project_id, environment_id, ingested_at, id);
CREATE INDEX security_sources_last_seen_idx ON security_sources (project_id, environment_id, last_seen_at DESC);
CREATE INDEX security_detections_browse_idx ON security_detections (project_id, environment_id, created_at DESC, id DESC);
