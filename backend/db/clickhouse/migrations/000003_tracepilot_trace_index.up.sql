CREATE TABLE IF NOT EXISTS tracepilot_trace_index (
    project_id UUID,
    environment_id UUID,
    trace_id String,
    started_at DateTime64(9, 'UTC'),
    ended_at DateTime64(9, 'UTC')
)
ENGINE = MergeTree
PARTITION BY toStartOfWeek(started_at, 1, 'UTC')
ORDER BY (project_id, environment_id, trace_id, started_at)
