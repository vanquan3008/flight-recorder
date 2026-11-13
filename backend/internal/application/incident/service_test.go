package incidentapp

import (
	"context"
	"testing"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/incident"
)

type statusStoreStub struct {
	status string
}

func (stub *statusStoreStub) UpdateIncidentStatus(_ context.Context, _, _, _, status string, _ auth.Actor) (incident.Incident, error) {
	stub.status = status
	return incident.Incident{Status: status}, nil
}

func TestUpdateStatusNormalizesBeforePersistence(t *testing.T) {
	store := &statusStoreStub{}
	updated, err := New(store).UpdateStatus(context.Background(), UpdateStatusCommand{ProjectID: "project", EnvironmentID: "environment", IncidentID: "incident", Status: " resolved ", Actor: auth.Actor{UserID: "actor"}})
	if err != nil {
		t.Fatal(err)
	}
	if store.status != "RESOLVED" || updated.Status != "RESOLVED" {
		t.Fatalf("status = %q, updated = %q", store.status, updated.Status)
	}
}

func TestUpdateStatusRejectsInvalidCommand(t *testing.T) {
	store := &statusStoreStub{}
	_, err := New(store).UpdateStatus(context.Background(), UpdateStatusCommand{Status: "closed", Actor: auth.Actor{UserID: "actor"}})
	if err != ErrInvalidStatus {
		t.Fatalf("error = %v", err)
	}
	if store.status != "" {
		t.Fatalf("store received %q", store.status)
	}
}
