package main

import (
	"testing"

	"github.com/vanquan3008/tracepilot/internal/config"
)

func TestMockEventsAreStableAndScoped(t *testing.T) {
	ingestion := config.IngestionConfig{
		ProjectID:       "01900000-0000-7000-8000-000000000001",
		EnvironmentName: "development",
	}
	first := mockEvents(ingestion)
	second := mockEvents(ingestion)
	if len(first) != 8 || len(second) != len(first) {
		t.Fatalf("mock event count = %d and %d, want 8", len(first), len(second))
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
	if first[len(first)-1].Security == nil || first[len(first)-1].Security.Format != "syslog+cef" {
		t.Fatalf("security mock event was not normalized: %#v", first[len(first)-1].Security)
	}
	if first[5].ResourceAttributes["host.name"] != "demo-host-01" || first[5].ResourceAttributes["log.file.path"] != "/var/log/auth.log" {
		t.Fatalf("machine log metadata = %#v", first[5].ResourceAttributes)
	}
}
