-- name: ListSubscribedWebhookIDs :many
SELECT id FROM webhooks
WHERE project_id = @project_id AND enabled = true AND @event_type::text = ANY(events);

-- The deterministic ID absorbs duplicate processing of the same domain event.
-- name: EnqueueWebhookDelivery :exec
INSERT INTO webhook_deliveries (
    id, project_id, webhook_id, event_type, payload, status, attempt, max_attempts,
    next_attempt_at, created_at, updated_at
) VALUES (
    @id, @project_id, @webhook_id, @event_type, @payload, 'pending', 0, 3,
    now(), now(), now()
)
ON CONFLICT DO NOTHING;

-- A success is terminal; a failure returns to pending until max_attempts.
-- name: RecordDeliveryAttempt :exec
UPDATE webhook_deliveries
SET status = CASE
        WHEN @succeeded::boolean THEN 'succeeded'
        WHEN attempt + 1 >= max_attempts THEN 'failed'
        ELSE 'pending'
    END,
    attempt = attempt + 1,
    http_status = sqlc.narg(http_status),
    last_error = NULLIF(@last_error::text, ''),
    next_attempt_at = @next_attempt_at,
    updated_at = now()
WHERE project_id = @project_id AND id = @id AND webhook_id = @webhook_id;

-- Requests are capped at five minutes, so a lease older than six belongs to a
-- crashed process and the delivery is claimable again.
-- name: ClaimDueDeliveries :many
WITH claim AS (
    SELECT d.id FROM webhook_deliveries d
    WHERE (
            (d.status = 'pending' AND d.next_attempt_at <= now())
         OR (d.status = 'sending' AND d.updated_at < now() - interval '6 minutes')
          )
      AND EXISTS (
          SELECT 1 FROM webhooks w
          WHERE w.project_id = d.project_id AND w.id = d.webhook_id AND w.enabled = true
      )
    ORDER BY d.next_attempt_at, d.created_at, d.id
    FOR UPDATE SKIP LOCKED
    LIMIT sqlc.arg(row_limit)::integer
)
UPDATE webhook_deliveries d
SET status = 'sending', updated_at = now()
FROM claim c, webhooks w
WHERE d.id = c.id
  AND w.project_id = d.project_id AND w.id = d.webhook_id AND w.enabled = true
RETURNING d.id, d.webhook_id, d.event_type, d.status, d.attempt, d.max_attempts, d.http_status,
          COALESCE(d.last_error, '')::text AS last_error, d.next_attempt_at, d.created_at,
          d.project_id, w.url, w.secret, d.payload;
