package postgres

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

// CreateWebhook encrypts the signing secret before it reaches PostgreSQL. An
// empty secret asks the server to generate one and return it exactly once.
func (database *Database) CreateWebhook(ctx context.Context, projectID, _ string, params notificationdomain.CreateWebhookParams) (notificationdomain.IssuedWebhook, error) {
	if err := params.Validate(); err != nil {
		return notificationdomain.IssuedWebhook{}, err
	}
	if params.Secret == "" {
		var secretError error
		params.Secret, secretError = newWebhookSecret()
		if secretError != nil {
			return notificationdomain.IssuedWebhook{}, secretError
		}
	}
	encryptedSecret, encryptError := notificationdomain.EncryptSecret(database.webhookEncryptionKey, params.Secret)
	if encryptError != nil {
		return notificationdomain.IssuedWebhook{}, fmt.Errorf("encrypt webhook secret: %w", encryptError)
	}
	now := time.Now().UTC()
	id := telemetry.DeterministicID(now, "webhook", projectID, params.Name)
	_, execError := database.pool.Exec(ctx, `
		INSERT INTO webhooks (id, project_id, name, url, secret, events, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, true, $7, $7)`,
		id, projectID, params.Name, params.URL, encryptedSecret, params.Events, now)
	if execError != nil {
		if isUniqueViolation(execError) {
			return notificationdomain.IssuedWebhook{}, notificationdomain.ErrNameExists
		}
		return notificationdomain.IssuedWebhook{}, fmt.Errorf("create webhook: %w", execError)
	}
	return notificationdomain.IssuedWebhook{Webhook: notificationdomain.Webhook{
		ID: id, Name: params.Name, URL: params.URL, Events: params.Events,
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	}, Secret: params.Secret}, nil
}

func (database *Database) UpdateWebhook(ctx context.Context, projectID, webhookID string, params notificationdomain.UpdateWebhookParams) (notificationdomain.Webhook, error) {
	if err := params.Validate(); err != nil {
		return notificationdomain.Webhook{}, err
	}
	var webhook notificationdomain.Webhook
	var queryError error
	if params.Enabled == nil {
		queryError = database.pool.QueryRow(ctx, `UPDATE webhooks SET name=$3,url=$4,events=$5,updated_at=now() WHERE project_id=$1 AND id=$2 RETURNING id,name,url,events,enabled,created_at,updated_at`, projectID, webhookID, params.Name, params.URL, params.Events).Scan(&webhook.ID, &webhook.Name, &webhook.URL, &webhook.Events, &webhook.Enabled, &webhook.CreatedAt, &webhook.UpdatedAt)
	} else {
		queryError = database.pool.QueryRow(ctx, `UPDATE webhooks SET name=$3,url=$4,events=$5,enabled=$6,updated_at=now() WHERE project_id=$1 AND id=$2 RETURNING id,name,url,events,enabled,created_at,updated_at`, projectID, webhookID, params.Name, params.URL, params.Events, *params.Enabled).Scan(&webhook.ID, &webhook.Name, &webhook.URL, &webhook.Events, &webhook.Enabled, &webhook.CreatedAt, &webhook.UpdatedAt)
	}
	if errors.Is(queryError, pgx.ErrNoRows) {
		return notificationdomain.Webhook{}, notificationdomain.ErrWebhookNotFound
	}
	if queryError != nil {
		if isUniqueViolation(queryError) {
			return notificationdomain.Webhook{}, notificationdomain.ErrNameExists
		}
		return notificationdomain.Webhook{}, fmt.Errorf("update webhook: %w", queryError)
	}
	return webhook, nil
}

// ListWebhooks returns paginated webhook metadata. Secret material is never
// selected, so accidental JSON serialization cannot disclose it.
func (database *Database) ListWebhooks(ctx context.Context, projectID string, filter notificationdomain.WebhookFilter) ([]notificationdomain.Webhook, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT id,name,url,events,enabled,created_at,updated_at FROM webhooks
		WHERE project_id=$1 AND ($2::timestamptz IS NULL OR created_at < $2 OR (created_at = $2 AND id < $3::uuid))
		ORDER BY created_at DESC,id DESC LIMIT $4`, projectID, nullableTime(filter.Before), nullableString(filter.CursorID), filter.Limit+1)
	if queryError != nil {
		return nil, fmt.Errorf("list webhooks: %w", queryError)
	}
	defer rows.Close()
	webhooks := make([]notificationdomain.Webhook, 0, filter.Limit+1)
	for rows.Next() {
		var webhook notificationdomain.Webhook
		if scanError := rows.Scan(&webhook.ID, &webhook.Name, &webhook.URL, &webhook.Events, &webhook.Enabled, &webhook.CreatedAt, &webhook.UpdatedAt); scanError != nil {
			return nil, fmt.Errorf("scan webhook: %w", scanError)
		}
		webhooks = append(webhooks, webhook)
	}
	return webhooks, rows.Err()
}

func (database *Database) GetWebhook(ctx context.Context, projectID, webhookID string) (notificationdomain.Webhook, error) {
	var webhook notificationdomain.Webhook
	queryError := database.pool.QueryRow(ctx, `SELECT id,name,url,events,enabled,created_at,updated_at FROM webhooks WHERE project_id=$1 AND id=$2`, projectID, webhookID).Scan(&webhook.ID, &webhook.Name, &webhook.URL, &webhook.Events, &webhook.Enabled, &webhook.CreatedAt, &webhook.UpdatedAt)
	if errors.Is(queryError, pgx.ErrNoRows) {
		return notificationdomain.Webhook{}, notificationdomain.ErrWebhookNotFound
	}
	if queryError != nil {
		return notificationdomain.Webhook{}, fmt.Errorf("get webhook: %w", queryError)
	}
	return webhook, nil
}

// DeleteWebhook removes a webhook; its delivery history is intentionally
// cascaded because it contains the outbound payload.
func (database *Database) DeleteWebhook(ctx context.Context, projectID, webhookID string) (bool, error) {
	result, execError := database.pool.Exec(ctx, `DELETE FROM webhooks WHERE project_id=$1 AND id=$2`, projectID, webhookID)
	if execError != nil {
		return false, fmt.Errorf("delete webhook: %w", execError)
	}
	return result.RowsAffected() == 1, nil
}

func (database *Database) ListWebhookDeliveries(ctx context.Context, projectID string, filter notificationdomain.DeliveryFilter) ([]notificationdomain.Delivery, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT id,webhook_id,event_type,status,attempt,max_attempts,http_status,COALESCE(last_error,''),next_attempt_at,created_at
		FROM webhook_deliveries WHERE project_id=$1 AND ($2='' OR webhook_id=$2::uuid)
		AND ($3::timestamptz IS NULL OR created_at < $3 OR (created_at = $3 AND id < $4::uuid))
		ORDER BY created_at DESC,id DESC LIMIT $5`, projectID, filter.WebhookID, nullableTime(filter.Before), nullableString(filter.CursorID), filter.Limit+1)
	if queryError != nil {
		return nil, fmt.Errorf("list webhook deliveries: %w", queryError)
	}
	defer rows.Close()
	deliveries := make([]notificationdomain.Delivery, 0, filter.Limit+1)
	for rows.Next() {
		var delivery notificationdomain.Delivery
		if scanError := rows.Scan(&delivery.ID, &delivery.WebhookID, &delivery.EventType, &delivery.Status, &delivery.Attempt, &delivery.MaxAttempts, &delivery.HTTPStatus, &delivery.LastError, &delivery.NextAttemptAt, &delivery.CreatedAt); scanError != nil {
			return nil, fmt.Errorf("scan delivery: %w", scanError)
		}
		deliveries = append(deliveries, delivery)
	}
	return deliveries, rows.Err()
}

func newWebhookSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate webhook secret: %w", err)
	}
	return "whsec_" + base64.RawURLEncoding.EncodeToString(bytes), nil
}

func (database *Database) RotateWebhookSecret(ctx context.Context, projectID, webhookID string) (string, error) {
	rawSecret, secretError := newWebhookSecret()
	if secretError != nil {
		return "", secretError
	}
	encryptedSecret, encryptError := notificationdomain.EncryptSecret(database.webhookEncryptionKey, rawSecret)
	if encryptError != nil {
		return "", fmt.Errorf("encrypt webhook secret: %w", encryptError)
	}
	result, execError := database.pool.Exec(ctx, `UPDATE webhooks SET secret=$3,updated_at=now() WHERE project_id=$1 AND id=$2`, projectID, webhookID, encryptedSecret)
	if execError != nil {
		return "", fmt.Errorf("rotate webhook secret: %w", execError)
	}
	if result.RowsAffected() == 0 {
		return "", notificationdomain.ErrWebhookNotFound
	}
	return rawSecret, nil
}
