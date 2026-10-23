package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func webhookDeliveryID(projectID, webhookID, eventType string, payload []byte) string {
	payloadDigest := sha256.Sum256(payload)
	// The payload digest makes notification emission idempotent across retries
	// and restarts. A fixed epoch is intentional: created_at remains the real
	// ordering field exposed to operators.
	return telemetry.DeterministicID(time.Unix(0, 0), "webhook-delivery", projectID, webhookID, eventType, hex.EncodeToString(payloadDigest[:]))
}

// enqueueWebhookDeliveries is called inside the same transaction that creates
// the incident/detection. Thus a committed domain event cannot be committed
// without its durable notification intent, while duplicate event processing is
// absorbed by the deterministic delivery key.
func enqueueWebhookDeliveries(ctx context.Context, transaction pgx.Tx, projectID, eventType string, payload any) error {
	encoded, marshalError := json.Marshal(map[string]any{"event": eventType, "payload": payload})
	if marshalError != nil {
		return fmt.Errorf("marshal webhook payload: %w", marshalError)
	}
	rows, queryError := transaction.Query(ctx, `SELECT id FROM webhooks WHERE project_id=$1 AND enabled=true AND $2=ANY(events)`, projectID, eventType)
	if queryError != nil {
		return fmt.Errorf("list transaction webhooks: %w", queryError)
	}
	// CollectRows closes the result before another command uses this transaction.
	webhookIDs, collectError := pgx.CollectRows(rows, pgx.RowTo[string])
	if collectError != nil {
		return fmt.Errorf("scan transaction webhooks: %w", collectError)
	}
	for _, webhookID := range webhookIDs {
		deliveryID := webhookDeliveryID(projectID, webhookID, eventType, encoded)
		if _, insertError := transaction.Exec(ctx, `
			INSERT INTO webhook_deliveries (id,project_id,webhook_id,event_type,payload,status,attempt,max_attempts,next_attempt_at,created_at,updated_at)
			VALUES ($1,$2,$3,$4,$5,'pending',0,3,now(),now(),now()) ON CONFLICT (project_id,id) DO NOTHING`,
			deliveryID, projectID, webhookID, eventType, json.RawMessage(encoded)); insertError != nil {
			return fmt.Errorf("enqueue transaction webhook: %w", insertError)
		}
	}
	return nil
}

// RecordDeliveryAttempt makes a successful delivery terminal and puts a
// failed delivery back into pending until max_attempts is reached.
func (database *Database) RecordDeliveryAttempt(ctx context.Context, deliveryID, webhookID, projectID string, httpStatus int, lastError string, success bool) error {
	nextAttempt := time.Now().UTC().Add(30 * time.Second)
	if success {
		nextAttempt = time.Now().UTC()
	}
	_, execError := database.pool.Exec(ctx, `
		UPDATE webhook_deliveries
		SET status = CASE WHEN $3 THEN 'succeeded' WHEN attempt + 1 >= max_attempts THEN 'failed' ELSE 'pending' END,
		    attempt = attempt + 1, http_status = $4, last_error = NULLIF($5, ''),
		    next_attempt_at = $6, updated_at = now()
		WHERE project_id = $1 AND id = $2 AND webhook_id = $7`,
		projectID, deliveryID, success, nullableInt(httpStatus), lastError, nextAttempt, webhookID)
	if execError != nil {
		return fmt.Errorf("record delivery attempt: %w", execError)
	}
	return nil
}

// ClaimDueDeliveries atomically leases pending work. Stale leases are reset so
// a process crash cannot leave a notification permanently stuck in sending.
func (database *Database) ClaimDueDeliveries(ctx context.Context, limit int) ([]notificationdomain.DeliveryAttempt, error) {
	if limit < 1 {
		return nil, errors.New("delivery claim limit must be positive")
	}
	transaction, beginError := database.pool.Begin(ctx)
	if beginError != nil {
		return nil, fmt.Errorf("begin claim webhook deliveries: %w", beginError)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	// Requests are capped at five minutes; leave a minute to record their result.
	if _, resetError := transaction.Exec(ctx, `UPDATE webhook_deliveries SET status='pending',next_attempt_at=now() WHERE status='sending' AND updated_at < now() - interval '6 minutes'`); resetError != nil {
		return nil, fmt.Errorf("reset stale webhook leases: %w", resetError)
	}
	rows, queryError := transaction.Query(ctx, `
		WITH claim AS (
			SELECT d.id FROM webhook_deliveries d
			WHERE d.status='pending' AND d.next_attempt_at <= now()
			  AND EXISTS (SELECT 1 FROM webhooks w WHERE w.project_id=d.project_id AND w.id=d.webhook_id AND w.enabled=true)
			ORDER BY d.next_attempt_at,d.created_at,d.id FOR UPDATE SKIP LOCKED LIMIT $1
		)
		UPDATE webhook_deliveries d SET status='sending',updated_at=now()
		FROM claim c
		JOIN webhook_deliveries claimed ON claimed.id=c.id
		JOIN webhooks w ON w.project_id=claimed.project_id AND w.id=claimed.webhook_id
		WHERE d.id=c.id AND w.enabled=true
		RETURNING d.id,d.webhook_id,d.event_type,d.status,d.attempt,d.max_attempts,d.http_status,
		          COALESCE(d.last_error,''),d.next_attempt_at,d.created_at,d.project_id,w.url,w.secret,d.payload`, limit)
	if queryError != nil {
		return nil, fmt.Errorf("claim webhook deliveries: %w", queryError)
	}
	defer rows.Close()
	claimed := make([]notificationdomain.DeliveryAttempt, 0, limit)
	for rows.Next() {
		var attempt notificationdomain.DeliveryAttempt
		var encryptedSecret string
		if scanError := rows.Scan(&attempt.ID, &attempt.WebhookID, &attempt.EventType, &attempt.Status, &attempt.Attempt, &attempt.MaxAttempts, &attempt.HTTPStatus, &attempt.LastError, &attempt.NextAttemptAt, &attempt.CreatedAt, &attempt.ProjectID, &attempt.URL, &encryptedSecret, &attempt.Payload); scanError != nil {
			return nil, fmt.Errorf("scan claimed webhook delivery: %w", scanError)
		}
		secret, decryptError := notificationdomain.DecryptSecret(database.webhookEncryptionKey, encryptedSecret)
		if decryptError != nil {
			return nil, fmt.Errorf("decrypt claimed webhook secret: %w", decryptError)
		}
		attempt.Secret = secret
		claimed = append(claimed, attempt)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate claimed webhook deliveries: %w", rowsError)
	}
	if commitError := transaction.Commit(ctx); commitError != nil {
		return nil, fmt.Errorf("commit webhook delivery claims: %w", commitError)
	}
	return claimed, nil
}
