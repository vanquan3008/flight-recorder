-- Webhook notification delivery: operators register HTTPS endpoints and select
-- which event types trigger a delivery attempt. Deliveries are retried up to
-- max_attempts times by the durable notification worker.
CREATE TABLE webhooks (
    id           uuid PRIMARY KEY,
    project_id   uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name         text NOT NULL,
    url          text NOT NULL,
    secret       text NOT NULL,          -- HMAC-SHA256 signing secret (protected database value)
    events       text[] NOT NULL,        -- e.g. ['detection.created', 'incident.opened']
    enabled      bool NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, name),
    UNIQUE (project_id, id),
    CONSTRAINT webhooks_url_format   CHECK (url   LIKE 'https://%'),
    CONSTRAINT webhooks_events_nonempty CHECK (array_length(events, 1) > 0)
);

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

CREATE INDEX webhooks_project_idx ON webhooks (project_id, enabled);
CREATE INDEX webhook_deliveries_pending_idx
    ON webhook_deliveries (project_id, next_attempt_at)
    WHERE status = 'pending';
CREATE INDEX webhook_deliveries_webhook_idx
    ON webhook_deliveries (project_id, webhook_id, created_at DESC, id DESC);
