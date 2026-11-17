package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func failingSpan(scope config.IngestionConfig, at time.Time, spanID, message string) telemetry.Event {
	event := flowSpan(scope, "api", at, spanID, "")
	event.Status, event.ErrorMessage = "ERROR", message
	return event
}

func TestIncidentBatchGroupsOccurrencesAndIsIdempotent(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Second).Add(-3 * time.Hour)
	userOne := failingSpan(scope, at, "1111111111111111", "user 1 not found")
	batch := []telemetry.Event{
		userOne,
		failingSpan(scope, at.Add(time.Minute), "2222222222222222", "user 2 not found"),
		userOne, // duplicate delivery inside one batch
		failingSpan(scope, at.Add(2*time.Minute), "3333333333333333", "connection refused"),
		failingSpan(scope, at.Add(2*time.Hour), "4444444444444444", "user 3 not found"), // outside the window
	}
	for attempt := range 2 { // the second attempt models an exporter retry
		if err := database.StoreEvents(ctx, scope, batch); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}

	rows, err := database.pool.Query(ctx, `
		SELECT error_message, occurrence_count, first_seen_at, last_seen_at
		FROM incidents WHERE project_id = $1 ORDER BY first_seen_at, error_message`, scope.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	type summary struct {
		message     string
		occurrences int64
		first, last time.Time
	}
	var got []summary
	for rows.Next() {
		var row summary
		if err := rows.Scan(&row.message, &row.occurrences, &row.first, &row.last); err != nil {
			t.Fatal(err)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []summary{
		{"user 2 not found", 2, at, at.Add(time.Minute)},
		{"connection refused", 1, at.Add(2 * time.Minute), at.Add(2 * time.Minute)},
		{"user 3 not found", 1, at.Add(2 * time.Hour), at.Add(2 * time.Hour)},
	}
	if len(got) != len(want) {
		t.Fatalf("incidents = %+v, want %+v", got, want)
	}
	for index := range want {
		if got[index].message != want[index].message || got[index].occurrences != want[index].occurrences ||
			!got[index].first.Equal(want[index].first) || !got[index].last.Equal(want[index].last) {
			t.Fatalf("incident %d = %+v, want %+v", index, got[index], want[index])
		}
	}

	var occurrences int
	if err := database.pool.QueryRow(ctx, "SELECT count(*) FROM incident_occurrences WHERE project_id = $1", scope.ProjectID).Scan(&occurrences); err != nil {
		t.Fatal(err)
	}
	if occurrences != 4 {
		t.Fatalf("occurrences = %d, want 4", occurrences)
	}
}

func TestStoreEventsMarksContentRejectedByPostgreSQL(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	event := flowSpan(scope, "api", time.Now().UTC(), "5555555555555555", "")
	event.Attributes = map[string]any{"body": "bad\x00body"} // jsonb rejects NUL: SQLSTATE class 22
	err := database.StoreEvents(context.Background(), scope, []telemetry.Event{event})
	if !errors.Is(err, telemetry.ErrInvalidEvent) {
		t.Fatalf("error = %v, want telemetry.ErrInvalidEvent", err)
	}
}
