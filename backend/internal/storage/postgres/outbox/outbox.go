package outbox

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
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

func webhookDeliveryID(projectID, webhookID, eventType string, payload []byte) string {
	payloadDigest := sha256.Sum256(payload)
	// The payload digest makes notification emission idempotent across retries
	// and restarts. A fixed epoch is intentional: created_at remains the real
	// ordering field exposed to operators.
	return telemetry.DeterministicID(time.Unix(0, 0), "webhook-delivery", projectID, webhookID, eventType, hex.EncodeToString(payloadDigest[:]))
}

// EnqueueDeliveries is called inside the same transaction that creates
// the incident/detection. Thus a committed domain event cannot be committed
// without its durable notification intent, while duplicate event processing is
// absorbed by the deterministic delivery key.
func EnqueueDeliveries(ctx context.Context, transaction pgx.Tx, projectID, eventType string, payload any) error {
	encoded, marshalError := json.Marshal(map[string]any{"event": eventType, "payload": payload})
	if marshalError != nil {
		return fmt.Errorf("marshal webhook payload: %w", marshalError)
	}
	queries := sqlcstore.New(transaction)
	webhookIDs, queryError := queries.ListSubscribedWebhookIDs(ctx, sqlcstore.ListSubscribedWebhookIDsParams{ProjectID: projectID, EventType: eventType})
	if queryError != nil {
		return fmt.Errorf("list subscribed webhooks: %w", queryError)
	}
	for _, webhookID := range webhookIDs {
		if insertError := queries.EnqueueWebhookDelivery(ctx, sqlcstore.EnqueueWebhookDeliveryParams{
			ID:        webhookDeliveryID(projectID, webhookID, eventType, encoded),
			ProjectID: projectID, WebhookID: webhookID, EventType: eventType, Payload: encoded,
		}); insertError != nil {
			return fmt.Errorf("enqueue webhook delivery: %w", insertError)
		}
	}
	return nil
}

// RecordDeliveryAttempt makes a successful delivery terminal and puts a
// failed delivery back into pending until max_attempts is reached.
func (store *OutboxStore) RecordDeliveryAttempt(ctx context.Context, deliveryID, webhookID, projectID string, httpStatus int, lastError string, success bool) error {
	nextAttempt := time.Now().UTC().Add(30 * time.Second)
	if success {
		nextAttempt = time.Now().UTC()
	}
	if execError := store.queries.RecordDeliveryAttempt(ctx, sqlcstore.RecordDeliveryAttemptParams{
		ProjectID: projectID, ID: deliveryID, WebhookID: webhookID, Succeeded: success,
		HTTPStatus: pgvalue.OrNil(int32(httpStatus)), LastError: lastError, NextAttemptAt: nextAttempt,
	}); execError != nil {
		return fmt.Errorf("record delivery attempt: %w", execError)
	}
	return nil
}

// ClaimDueDeliveries atomically leases due work, including deliveries whose
// lease expired because the process holding it crashed.
func (store *OutboxStore) ClaimDueDeliveries(ctx context.Context, limit int) ([]notificationdomain.DeliveryAttempt, error) {
	if limit < 1 {
		return nil, errors.New("delivery claim limit must be positive")
	}
	var claimed []notificationdomain.DeliveryAttempt
	// Secrets are decrypted before commit: a failure rolls the lease back
	// instead of leaving deliveries stuck in sending until the lease expires.
	claimError := pgx.BeginFunc(ctx, store.pool, func(transaction pgx.Tx) error {
		queries := store.queries.WithTx(transaction)
		rows, queryError := queries.ClaimDueDeliveries(ctx, int32(limit))
		if queryError != nil {
			return queryError
		}
		claimed = make([]notificationdomain.DeliveryAttempt, 0, len(rows))
		for _, row := range rows {
			secret, decryptError := notificationdomain.DecryptSecret(store.encryptionKey, row.Secret)
			if decryptError != nil {
				return fmt.Errorf("decrypt claimed webhook secret: %w", decryptError)
			}
			claimed = append(claimed, notificationdomain.DeliveryAttempt{
				Delivery: notificationdomain.Delivery{
					ID: row.ID, WebhookID: row.WebhookID, EventType: row.EventType, Status: row.Status,
					Attempt: int(row.Attempt), MaxAttempts: int(row.MaxAttempts), HTTPStatus: pgvalue.IntPointer(row.HTTPStatus),
					LastError: row.LastError, NextAttemptAt: row.NextAttemptAt, CreatedAt: row.CreatedAt,
				},
				ProjectID: row.ProjectID, URL: row.URL, Secret: secret, Payload: row.Payload,
			})
		}
		return nil
	})
	if claimError != nil {
		return nil, fmt.Errorf("claim webhook deliveries: %w", claimError)
	}
	return claimed, nil
}
