package postgres

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func TestWebhookEncryptionAndDurableClaimIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("TRACEPILOT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TRACEPILOT_DATABASE_URL is required")
	}
	key := []byte(strings.Repeat("w", 32))
	database, openError := OpenWithWebhookEncryptionKey(context.Background(), config.DatabaseConfig{URL: databaseURL, MaxConnections: 4, MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute}, hex.EncodeToString(key))
	if openError != nil {
		t.Fatal(openError)
	}
	defer database.Close()
	runID := time.Now().UTC().Format(time.RFC3339Nano)
	projectID := telemetry.DeterministicID(time.Now().UTC(), "webhook-integration-project", runID)
	ingestion := config.IngestionConfig{ProjectID: projectID, ProjectSlug: "webhook-" + projectID[24:32], ProjectName: "Webhook Integration", EnvironmentID: telemetry.DeterministicID(time.Now().UTC(), "webhook-integration-environment", runID), EnvironmentName: "test"}
	if err := database.EnsureIngestionScope(context.Background(), ingestion); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.pool.Exec(context.Background(), "DELETE FROM webhooks WHERE project_id=$1", projectID)
	})
	issued, createError := database.CreateWebhook(context.Background(), projectID, "", notificationdomain.CreateWebhookParams{Name: "integration", URL: "https://example.com/tracepilot", Events: []string{"incident.opened"}})
	if createError != nil {
		t.Fatal(createError)
	}
	var stored string
	if err := database.pool.QueryRow(context.Background(), "SELECT secret FROM webhooks WHERE project_id=$1 AND id=$2", projectID, issued.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == issued.Secret || !strings.HasPrefix(stored, "v1:") {
		t.Fatalf("secret is not encrypted: %q", stored)
	}
	payload := []byte(`{"event":"incident.opened","payload":{"id":"one"}}`)
	deliveryID := telemetry.DeterministicID(time.Now().UTC(), "webhook-integration-delivery", runID)
	if _, err := database.pool.Exec(context.Background(), `INSERT INTO webhook_deliveries (id,project_id,webhook_id,event_type,payload,status,attempt,max_attempts,next_attempt_at,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,'pending',0,3,now(),now(),now()) ON CONFLICT (project_id,id) DO NOTHING`, deliveryID, projectID, issued.ID, "incident.opened", json.RawMessage(payload)); err != nil {
		t.Fatal(err)
	}
	claimed, err := database.ClaimDueDeliveries(context.Background(), 1000)
	if err != nil || len(claimed) == 0 {
		t.Fatalf("claimed=%#v error=%v", claimed, err)
	}
	var found *notificationdomain.DeliveryAttempt
	for index := range claimed {
		if claimed[index].ID == deliveryID {
			found = &claimed[index]
			break
		}
	}
	if found == nil {
		t.Fatalf("claimed delivery does not contain decrypted delivery data: found=%#v wantID=%s wantSecret=%s wantPayload=%s all=%#v", found, deliveryID, issued.Secret, payload, claimed)
	}
	var compactPayload, compactFound bytes.Buffer
	_ = json.Compact(&compactPayload, payload)
	_ = json.Compact(&compactFound, found.Payload)
	if found.Secret != issued.Secret || compactFound.String() != compactPayload.String() {
		t.Fatalf("claimed delivery does not contain decrypted delivery data: found=%#v wantID=%s wantSecret=%s wantPayload=%s all=%#v", found, deliveryID, issued.Secret, payload, claimed)
	}
	if err := database.RecordDeliveryAttempt(context.Background(), deliveryID, issued.ID, projectID, 503, "unavailable", false); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := database.pool.QueryRow(context.Background(), "SELECT status FROM webhook_deliveries WHERE project_id=$1 AND id=$2", projectID, deliveryID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("status after first failure = %q, want pending", status)
	}
}
