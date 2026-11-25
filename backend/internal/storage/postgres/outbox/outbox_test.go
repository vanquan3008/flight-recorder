package outbox

import (
	"context"
	"testing"
)

func TestWebhookDeliveryIDFollowsTheDomainEvent(t *testing.T) {
	const project, webhook = "6f1c2f5e-0c1d-4c1a-9f37-0a9b0c7d2e11", "0b6a3c2d-5e4f-4a1b-8c9d-1e2f3a4b5c6d"
	first := webhookDeliveryID(project, webhook, "incident.resolved", "change-1")
	if again := webhookDeliveryID(project, webhook, "incident.resolved", "change-1"); again != first {
		t.Fatalf("the same event produced delivery IDs %s and %s; a retry would notify twice", first, again)
	}
	// Two resolves by the same operator carry identical payloads; only the
	// status change distinguishes them, and each must notify.
	if second := webhookDeliveryID(project, webhook, "incident.resolved", "change-2"); second == first {
		t.Fatal("two status changes share a delivery ID; the second notification would be dropped")
	}
	if other := webhookDeliveryID(project, "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d", "incident.resolved", "change-1"); other == first {
		t.Fatal("two webhooks share a delivery ID for one event")
	}
}

func TestEnqueueDeliveriesRequiresAnEventKey(t *testing.T) {
	// The key check runs before the transaction is used.
	if err := EnqueueDeliveries(context.Background(), nil, "project", "incident.opened", "", map[string]any{}); err == nil {
		t.Fatal("a delivery without an event key would fall back to an ambiguous identity")
	}
}
