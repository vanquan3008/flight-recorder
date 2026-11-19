CREATE TABLE IF NOT EXISTS tracepilot_span_minute_done (
    project_id UUID,
    environment_id UUID,
    minute DateTime('UTC'),
    computed_at DateTime64(3, 'UTC')
)
ENGINE = ReplacingMergeTree(computed_at)
PARTITION BY toStartOfWeek(minute, 1, 'UTC')
ORDER BY (project_id, environment_id, minute)
