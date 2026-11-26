package webhooks

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
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// CreateWebhook encrypts the signing secret before it reaches PostgreSQL. An
// empty secret asks the server to generate one and return it exactly once.
func (store *WebhookStore) CreateWebhook(ctx context.Context, projectID, _ string, params notificationdomain.CreateWebhookParams) (notificationdomain.IssuedWebhook, error) {
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
	encryptedSecret, encryptError := notificationdomain.EncryptSecret(store.encryptionKey, params.Secret)
	if encryptError != nil {
		return notificationdomain.IssuedWebhook{}, fmt.Errorf("encrypt webhook secret: %w", encryptError)
	}
	now := time.Now().UTC()
	id := telemetry.DeterministicID(now, "webhook", projectID, params.Name)
	insertError := store.queries.InsertWebhook(ctx, sqlcstore.InsertWebhookParams{
		ID: id, ProjectID: projectID, Name: params.Name, URL: params.URL,
		Secret: encryptedSecret, Events: params.Events, CreatedAt: now,
	})
	if insertError != nil {
		if pgvalue.IsUniqueViolation(insertError) {
			return notificationdomain.IssuedWebhook{}, notificationdomain.ErrNameExists
		}
		return notificationdomain.IssuedWebhook{}, fmt.Errorf("create webhook: %w", insertError)
	}
	return notificationdomain.IssuedWebhook{Webhook: notificationdomain.Webhook{
		ID: id, Name: params.Name, URL: params.URL, Events: params.Events,
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	}, Secret: params.Secret}, nil
}

func (store *WebhookStore) UpdateWebhook(ctx context.Context, projectID, webhookID string, params notificationdomain.UpdateWebhookParams) (notificationdomain.Webhook, error) {
	if err := params.Validate(); err != nil {
		return notificationdomain.Webhook{}, err
	}
	row, queryError := store.queries.UpdateWebhook(ctx, sqlcstore.UpdateWebhookParams{
		ProjectID: projectID, ID: webhookID, Name: params.Name, URL: params.URL,
		Events: params.Events, Enabled: params.Enabled,
	})
	if errors.Is(queryError, pgx.ErrNoRows) {
		return notificationdomain.Webhook{}, notificationdomain.ErrWebhookNotFound
	}
	if queryError != nil {
		if pgvalue.IsUniqueViolation(queryError) {
			return notificationdomain.Webhook{}, notificationdomain.ErrNameExists
		}
		return notificationdomain.Webhook{}, fmt.Errorf("update webhook: %w", queryError)
	}
	return webhook(sqlcstore.GetWebhookRow(row)), nil
}

// ListWebhooks returns paginated webhook metadata. Secret material is never
// selected, so accidental JSON serialization cannot disclose it.
func (store *WebhookStore) ListWebhooks(ctx context.Context, projectID string, filter notificationdomain.WebhookFilter) ([]notificationdomain.Webhook, error) {
	rows, queryError := store.queries.ListWebhooks(ctx, sqlcstore.ListWebhooksParams{
		ProjectID: projectID,
		Before:    pgvalue.TimeOrNil(filter.Before),
		CursorID:  pgvalue.OrNil(filter.CursorID),
		RowLimit:  int32(filter.Limit + 1),
	})
	if queryError != nil {
		return nil, fmt.Errorf("list webhooks: %w", queryError)
	}
	webhooks := make([]notificationdomain.Webhook, 0, len(rows))
	for _, row := range rows {
		webhooks = append(webhooks, webhook(sqlcstore.GetWebhookRow(row)))
	}
	return webhooks, nil
}

func (store *WebhookStore) GetWebhook(ctx context.Context, projectID, webhookID string) (notificationdomain.Webhook, error) {
	row, queryError := store.queries.GetWebhook(ctx, sqlcstore.GetWebhookParams{ProjectID: projectID, ID: webhookID})
	if errors.Is(queryError, pgx.ErrNoRows) {
		return notificationdomain.Webhook{}, notificationdomain.ErrWebhookNotFound
	}
	if queryError != nil {
		return notificationdomain.Webhook{}, fmt.Errorf("get webhook: %w", queryError)
	}
	return webhook(row), nil
}

// DeleteWebhook removes a webhook; its delivery history is intentionally
// cascaded because it contains the outbound payload.
func (store *WebhookStore) DeleteWebhook(ctx context.Context, projectID, webhookID string) (bool, error) {
	deleted, deleteError := store.queries.DeleteWebhook(ctx, sqlcstore.DeleteWebhookParams{ProjectID: projectID, ID: webhookID})
	if deleteError != nil {
		return false, fmt.Errorf("delete webhook: %w", deleteError)
	}
	return deleted == 1, nil
}

func (store *WebhookStore) ListWebhookDeliveries(ctx context.Context, projectID string, filter notificationdomain.DeliveryFilter) ([]notificationdomain.Delivery, error) {
	rows, queryError := store.queries.ListWebhookDeliveries(ctx, sqlcstore.ListWebhookDeliveriesParams{
		ProjectID: projectID,
		WebhookID: pgvalue.OrNil(filter.WebhookID),
		Before:    pgvalue.TimeOrNil(filter.Before),
		CursorID:  pgvalue.OrNil(filter.CursorID),
		RowLimit:  int32(filter.Limit + 1),
	})
	if queryError != nil {
		return nil, fmt.Errorf("list webhook deliveries: %w", queryError)
	}
	deliveries := make([]notificationdomain.Delivery, 0, len(rows))
	for _, row := range rows {
		deliveries = append(deliveries, notificationdomain.Delivery{
			ID: row.ID, WebhookID: row.WebhookID, EventType: row.EventType, Status: row.Status,
			Attempt: int(row.Attempt), MaxAttempts: int(row.MaxAttempts), HTTPStatus: pgvalue.IntPointer(row.HTTPStatus),
			LastError: row.LastError, NextAttemptAt: row.NextAttemptAt, CreatedAt: row.CreatedAt,
		})
	}
	return deliveries, nil
}

func newWebhookSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate webhook secret: %w", err)
	}
	return "whsec_" + base64.RawURLEncoding.EncodeToString(bytes), nil
}

func (store *WebhookStore) RotateWebhookSecret(ctx context.Context, projectID, webhookID string) (string, error) {
	rawSecret, secretError := newWebhookSecret()
	if secretError != nil {
		return "", secretError
	}
	encryptedSecret, encryptError := notificationdomain.EncryptSecret(store.encryptionKey, rawSecret)
	if encryptError != nil {
		return "", fmt.Errorf("encrypt webhook secret: %w", encryptError)
	}
	updated, updateError := store.queries.RotateWebhookSecret(ctx, sqlcstore.RotateWebhookSecretParams{
		ProjectID: projectID, ID: webhookID, Secret: encryptedSecret,
	})
	if updateError != nil {
		return "", fmt.Errorf("rotate webhook secret: %w", updateError)
	}
	if updated == 0 {
		return "", notificationdomain.ErrWebhookNotFound
	}
	return rawSecret, nil
}

func webhook(row sqlcstore.GetWebhookRow) notificationdomain.Webhook {
	return notificationdomain.Webhook{
		ID: row.ID, Name: row.Name, URL: row.URL, Events: row.Events,
		Enabled: row.Enabled, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}
