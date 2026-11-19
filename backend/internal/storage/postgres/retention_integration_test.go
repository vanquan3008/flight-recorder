package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func retentionTestEvents(projectID, environmentName, label string, timestamp time.Time) (telemetry.Event, telemetry.Event) {
	spanEvent := telemetry.Event{
		ID: telemetry.DeterministicID(timestamp, label, "span"), ProjectID: projectID, Environment: environmentName,
		ServiceName: "retention-service", Type: telemetry.EventHTTPServer, Timestamp: timestamp, Duration: time.Second,
		TraceID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SpanID: "bbbbbbbbbbbbbbbb", Status: "ERROR", Signal: telemetry.SignalTrace,
		Attributes: map[string]any{}, ResourceAttributes: map[string]any{}, ScopeAttributes: map[string]any{}, RecordAttributes: map[string]any{},
	}
	logEvent := telemetry.Event{
		ID: telemetry.DeterministicID(timestamp, label, "log"), ProjectID: projectID, Environment: environmentName,
		ServiceName: "retention-service", Type: telemetry.EventLog, Timestamp: timestamp, Severity: "ERROR", ErrorType: "RetentionError",
		Signal: telemetry.SignalLog, Attributes: map[string]any{}, ResourceAttributes: map[string]any{}, ScopeAttributes: map[string]any{}, RecordAttributes: map[string]any{},
		LogBody: map[string]any{"message": label},
	}
	return spanEvent, logEvent
}

type retentionCounts struct{ events, occurrences, links, incidents int }

func countRetention(t *testing.T, database *Database, projectID string) retentionCounts {
	t.Helper()
	var counts retentionCounts
	if queryError := database.pool.QueryRow(context.Background(), `
		SELECT
			(SELECT count(*) FROM events WHERE project_id = $1),
			(SELECT count(*) FROM incident_occurrences WHERE project_id = $1),
			(SELECT count(*) FROM incident_events WHERE project_id = $1),
			(SELECT count(*) FROM incidents WHERE project_id = $1)`, projectID).Scan(
		&counts.events, &counts.occurrences, &counts.links, &counts.incidents); queryError != nil {
		t.Fatal(queryError)
	}
	return counts
}

func TestProjectRetentionPreservesSharedPartitionsAndInheritedPolicyIntegration(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2001, 2, 20, 12, 0, 30, 0, time.UTC)
	old, shared, fresh := now.AddDate(0, 0, -16), now.AddDate(0, 0, -9), now.AddDate(0, 0, -1)
	short, long := int32(1), int32(14)
	projects := []struct {
		name string
		days *int32
		kept int
	}{{"short", &short, 1}, {"long", &long, 2}, {"inherited", nil, 1}}
	worker := openIntegrationDatabase(t)
	t.Cleanup(worker.Close)
	if err := worker.EnsurePartitions(ctx, old, 17); err != nil {
		t.Fatal(err)
	}
	for _, project := range projects {
		database := openIntegrationDatabase(t)
		id := telemetry.DeterministicID(now, "retention-project", project.name)
		ingestion := config.IngestionConfig{ProjectID: id, ProjectSlug: "retention-" + project.name, ProjectName: project.name,
			EnvironmentID: telemetry.DeterministicID(now, "retention-environment", project.name), EnvironmentName: "test"}
		ensureIntegrationScope(t, database, ingestion)
		t.Cleanup(func() { cleanupIntegrationScope(t, database, id) })
		if project.days != nil {
			if err := database.UpdateProjectRetention(ctx, id, project.days, auth.Actor{}); err != nil {
				t.Fatal(err)
			}
		}
		for _, at := range []time.Time{old, shared, fresh} {
			span, _ := retentionTestEvents(id, ingestion.EnvironmentName, project.name, at)
			if err := database.StoreEvents(ctx, ingestion, []telemetry.Event{span}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if events, err := worker.DropExpiredTelemetryPartitions(ctx, now, 0); err != nil || events != 0 {
		t.Fatalf("indefinite inherited policy allowed global deletion: events=%d err=%v", events, err)
	}
	const defaultAge = 7 * 24 * time.Hour
	if events, err := worker.DropExpiredTelemetryPartitions(ctx, now, defaultAge); err != nil || events != 3 {
		t.Fatalf("shared expired partition: events=%d err=%v", events, err)
	}
	for _, project := range projects {
		id := telemetry.DeterministicID(now, "retention-project", project.name)
		if _, err := worker.PruneProjectTelemetry(ctx, id, now, defaultAge, 1); err != nil {
			t.Fatalf("prune %s: %v", project.name, err)
		}
		if counts := countRetention(t, worker, id); counts.events != project.kept {
			t.Fatalf("%s envelopes after cleanup = %+v, want %d", project.name, counts, project.kept)
		}
	}
}

// Retention drops whole daily partitions. Incident rows reference events
// without a foreign key, so they intentionally outlive the telemetry.
func TestRetentionDropsExpiredPartitionsIntegration(t *testing.T) {
	database := openIntegrationDatabase(t)
	projectID := "01900000-0000-7000-8000-000000000601"
	ingestion := config.IngestionConfig{
		ProjectID: projectID, ProjectSlug: "retention-integration", ProjectName: "Retention Integration",
		EnvironmentID: "01900000-0000-7000-8000-000000000602", EnvironmentName: "test",
	}
	ensureIntegrationScope(t, database, ingestion)
	t.Cleanup(func() { cleanupIntegrationScope(t, database, projectID) })
	ctx := context.Background()

	oldDay := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	keptDay := time.Date(2026, 1, 12, 12, 0, 0, 0, time.UTC)
	strayDay := time.Date(2025, 12, 1, 12, 0, 0, 0, time.UTC)
	if ensureError := database.EnsurePartitions(ctx, oldDay, 3); ensureError != nil {
		t.Fatal(ensureError)
	}
	// PostgreSQL keeps the incident envelope of each failed span and error log.
	var failures []telemetry.Event
	for _, day := range []struct {
		label string
		at    time.Time
	}{{"old", oldDay}, {"kept", keptDay}, {"stray", strayDay}} {
		span, log := retentionTestEvents(projectID, ingestion.EnvironmentName, day.label, day.at)
		failures = append(failures, span, log)
	}
	if storeError := database.StoreEvents(ctx, ingestion, failures); storeError != nil {
		t.Fatal(storeError)
	}
	before := countRetention(t, database, projectID)
	if before.events != 6 || before.incidents == 0 {
		t.Fatalf("seed counts = %+v", before)
	}

	cutoff := time.Date(2026, 1, 11, 12, 0, 0, 0, time.UTC)
	removed, deleteError := database.DeleteEventsBefore(ctx, cutoff, 100)
	if deleteError != nil {
		t.Fatal(deleteError)
	}
	// The 2026-01-10 partition ended before the cutoff (2 events); the stray
	// 2025-12-01 events sit in the default partition and are deleted by batch.
	if removed != 4 {
		t.Fatalf("removed events = %d, want 4", removed)
	}
	after := countRetention(t, database, projectID)
	if after.events != 2 {
		t.Fatalf("envelopes after retention = %+v, want only the kept day", after)
	}
	if after.occurrences != before.occurrences || after.links != before.links || after.incidents != before.incidents {
		t.Fatalf("incident rows changed by event retention: before=%+v after=%+v", before, after)
	}
	var partitionExists bool
	if queryError := database.pool.QueryRow(ctx, "SELECT to_regclass('events_p20260110') IS NOT NULL").Scan(&partitionExists); queryError != nil {
		t.Fatal(queryError)
	}
	if partitionExists {
		t.Fatal("expired events partition still exists")
	}
	if again, repeatError := database.DeleteEventsBefore(ctx, cutoff, 100); repeatError != nil || again != 0 {
		t.Fatalf("repeated retention removed %d events, error=%v", again, repeatError)
	}
}

func TestEnsurePartitionsToleratesRowsInDefaultPartitionIntegration(t *testing.T) {
	database := openIntegrationDatabase(t)
	projectID := "01900000-0000-7000-8000-000000000611"
	ingestion := config.IngestionConfig{
		ProjectID: projectID, ProjectSlug: "partition-integration", ProjectName: "Partition Integration",
		EnvironmentID: "01900000-0000-7000-8000-000000000612", EnvironmentName: "test",
	}
	ensureIntegrationScope(t, database, ingestion)
	t.Cleanup(func() { cleanupIntegrationScope(t, database, projectID) })
	ctx := context.Background()

	// This day has no partition yet, so the row lands in the default partition.
	strayDay := time.Date(2031, 3, 5, 9, 0, 0, 0, time.UTC)
	span, _ := retentionTestEvents(projectID, ingestion.EnvironmentName, "stray-default", strayDay)
	if storeError := database.StoreEvents(ctx, ingestion, []telemetry.Event{span}); storeError != nil {
		t.Fatalf("a missing partition must not reject ingestion: %v", storeError)
	}
	for range 2 {
		if ensureError := database.EnsurePartitions(ctx, strayDay, 2); ensureError != nil {
			t.Fatalf("EnsurePartitions failed with rows in the default partition: %v", ensureError)
		}
	}
	var dailyPartitions int
	if queryError := database.pool.QueryRow(ctx, "SELECT count(*) FROM pg_class WHERE relname IN ('events_p20310305', 'events_p20310306')").Scan(&dailyPartitions); queryError != nil {
		t.Fatal(queryError)
	}
	if dailyPartitions != 1 {
		t.Fatalf("daily partitions created = %d, want only the day without default-partition rows", dailyPartitions)
	}
	if counts := countRetention(t, database, projectID); counts.events != 1 {
		t.Fatalf("stray row lost: %+v", counts)
	}
}
