package main

import (
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
)

func TestMockEventsAreStableAndScoped(t *testing.T) {
	ingestion := config.IngestionConfig{
		ProjectID:       "01900000-0000-7000-8000-000000000001",
		EnvironmentName: "development",
	}
	now := time.Date(2026, 10, 9, 9, 2, 0, 0, time.UTC)
	first := mockEvents(ingestion, now)
	second := mockEvents(ingestion, now.Add(time.Minute))
	if len(first) != 6 || len(second) != len(first) {
		t.Fatalf("mock event count = %d and %d, want 6", len(first), len(second))
	}
	seen := make(map[string]struct{}, len(first))
	for index, event := range first {
		if event.ProjectID != ingestion.ProjectID || event.Environment != ingestion.EnvironmentName {
			t.Fatalf("event %d has wrong scope: %#v", index, event)
		}
		if _, exists := seen[event.ID]; exists {
			t.Fatalf("duplicate event ID %q", event.ID)
		}
		seen[event.ID] = struct{}{}
		if event.ID != second[index].ID {
			t.Fatalf("event %d is not stable across runs", index)
		}
	}
}

func TestMockEventsFitDefaultWindowAndHaveScopedIDs(t *testing.T) {
	scope := config.IngestionConfig{ProjectID: "one", EnvironmentID: "development"}
	for _, minute := range []int{0, 4, 5, 59} {
		now := time.Date(2026, 10, 9, 9, minute, 0, 0, time.UTC)
		for _, event := range mockEvents(scope, now) {
			if event.Timestamp.Before(now.Add(-15*time.Minute)) || event.Timestamp.After(now) {
				t.Fatalf("demo event is outside the default window: %s", event.Timestamp)
			}
		}
	}
	now := time.Date(2026, 10, 9, 9, 2, 0, 0, time.UTC)
	first := mockEvents(scope, now)[0]
	later := mockEvents(scope, now.Add(5*time.Minute))[0]
	if first.TraceID == later.TraceID || first.ID == later.ID {
		t.Fatal("new demo window reused span identity and would conflict with existing spans")
	}
	scope.ProjectID = "two"
	otherProject := mockEvents(scope, now)[0]
	scope.ProjectID, scope.EnvironmentID = "one", "production"
	otherEnvironment := mockEvents(scope, now)[0]
	if first.ID == otherProject.ID || first.ID == otherEnvironment.ID {
		t.Fatal("demo IDs overlap between scopes")
	}
}
