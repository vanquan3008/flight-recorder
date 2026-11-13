-- Ingest locks affected minutes in the same order as refresh. Retried spans
-- may mark a minute dirty again; rebuilding from stored spans is idempotent.
-- name: MarkSpanRollupBuckets :exec
INSERT INTO span_rollup_buckets (project_id, environment_id, bucket_start)
SELECT DISTINCT @project_id::uuid, @environment_id::uuid,
    date_bin('1 minute', times.started_at, TIMESTAMPTZ '2000-01-01 00:00:00+00')
FROM unnest(@started_ats::timestamptz[]) AS times(started_at)
ORDER BY 1, 2, 3
ON CONFLICT (project_id, environment_id, bucket_start) DO UPDATE SET dirty = true;

-- Hold these locks until RefreshSpanRollups commits. Its separate statement
-- gets a fresh snapshot, including ingests that committed before the locks.
-- name: LockPendingSpanRollupBuckets :many
SELECT bucket_start FROM span_rollup_buckets
WHERE project_id = @project_id AND environment_id = @environment_id AND dirty
ORDER BY bucket_start
LIMIT sqlc.arg(row_limit)::integer
FOR UPDATE SKIP LOCKED;

-- name: RefreshSpanRollups :exec
WITH removed AS (
    DELETE FROM span_minute_rollups
    WHERE project_id = @project_id AND environment_id = @environment_id
      AND bucket_start = ANY(@bucket_starts::timestamptz[])
    RETURNING 1
), refreshed AS (
    INSERT INTO span_minute_rollups (
        project_id, environment_id, bucket_start, service_name, span_kind, endpoint,
        entity_type, entity_name, calls, errors, duration_sum, trace_ids,
        request_started_at, request_duration_ns, request_failed
    )
    SELECT @project_id, @environment_id, minute.bucket_start, s.service_name, s.span_kind,
        CASE WHEN s.span_kind = 2 THEN span_request_endpoint(s.span_attributes, s.span_name) ELSE '' END,
        CASE WHEN span_is_dependency(s.span_attributes, s.span_kind) THEN span_dependency_type(s.span_attributes) ELSE '' END,
        CASE WHEN span_is_dependency(s.span_attributes, s.span_kind) THEN COALESCE(span_dependency_name(s.span_attributes), '') ELSE '' END,
        count(*), count(*) FILTER (WHERE s.status_code = 'ERROR'),
        sum(extract(epoch FROM s.ended_at - s.started_at) * 1000000000),
        (array_agg(DISTINCT s.trace_id ORDER BY s.trace_id))[1:5],
        COALESCE(array_agg(s.started_at ORDER BY s.started_at, s.event_id) FILTER (WHERE s.span_kind = 2), '{}'),
        COALESCE(array_agg((extract(epoch FROM s.ended_at - s.started_at) * 1000000000)::bigint
            ORDER BY s.started_at, s.event_id) FILTER (WHERE s.span_kind = 2), '{}'),
        COALESCE(array_agg(span_request_failed(s.status_code, s.span_attributes)
            ORDER BY s.started_at, s.event_id) FILTER (WHERE s.span_kind = 2), '{}')
    FROM unnest(@bucket_starts::timestamptz[]) AS minute(bucket_start)
    JOIN spans s ON s.project_id = @project_id AND s.environment_id = @environment_id
        AND s.started_at >= minute.bucket_start AND s.started_at < minute.bucket_start + interval '1 minute'
    CROSS JOIN (SELECT count(*) FROM removed) AS deletion
    GROUP BY minute.bucket_start, s.service_name, s.span_kind, 6, 7, 8
    RETURNING 1
)
UPDATE span_rollup_buckets AS b SET dirty = false
WHERE b.project_id = @project_id AND b.environment_id = @environment_id
  AND b.bucket_start = ANY(@bucket_starts::timestamptz[])
  AND (SELECT count(*) FROM refreshed) >= 0;

-- Retention also removes a partially expired minute: surviving spans use the
-- raw path until a new arrival schedules that minute again.
-- name: DeleteSpanRollupBucketsBefore :exec
DELETE FROM span_rollup_buckets WHERE bucket_start < @before_time::timestamptz;
