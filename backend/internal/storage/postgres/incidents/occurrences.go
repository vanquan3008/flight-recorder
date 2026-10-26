package incidents

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/outbox"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgbatch"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

const incidentDeduplicationWindow = time.Hour

// incidentOccurrences collects the new occurrences one batch adds to a single
// incident, so the incident row is written once instead of once per event.
type incidentOccurrences struct {
	id         string
	created    bool
	lastSeenAt time.Time
	events     []telemetry.Event // oldest first
}

// RecordOccurrences records the failing events of one ingest batch. Events are
// grouped by fingerprint, so an error storm costs a few statements per distinct
// failure rather than per event. Fingerprints are locked in sorted order, so two
// concurrent batches cannot deadlock on each other's advisory locks.
func RecordOccurrences(ctx context.Context, transaction pgx.Tx, ingestion config.IngestionConfig, events []telemetry.Event) error {
	groups := make(map[string][]telemetry.Event)
	for _, event := range events {
		if incident.Candidate(event) {
			fingerprint := incident.Fingerprint(event)
			groups[fingerprint] = append(groups[fingerprint], event)
		}
	}
	for _, fingerprint := range slices.Sorted(maps.Keys(groups)) {
		touched, storeError := storeFingerprintOccurrences(ctx, transaction, ingestion, fingerprint, groups[fingerprint])
		if storeError != nil {
			return storeError
		}
		for _, record := range touched {
			if !record.created {
				continue
			}
			opening := record.events[0]
			if enqueueError := outbox.EnqueueDeliveries(ctx, transaction, ingestion.ProjectID, "incident.opened", map[string]any{
				"incident_id": record.id, "environment_id": ingestion.EnvironmentID, "service": opening.ServiceName,
				"title": incident.Title(opening), "severity": incident.Severity(opening),
			}); enqueueError != nil {
				return enqueueError
			}
		}
	}
	return nil
}

// RecordOccurrence records a single event. incidentID is empty when an earlier
// attempt already recorded the event; created reports a newly opened incident.
func RecordOccurrence(ctx context.Context, transaction pgx.Tx, ingestion config.IngestionConfig, event telemetry.Event) (incidentID string, created bool, err error) {
	touched, storeError := storeFingerprintOccurrences(ctx, transaction, ingestion, incident.Fingerprint(event), []telemetry.Event{event})
	if storeError != nil || len(touched) == 0 {
		return "", false, storeError
	}
	return touched[0].id, touched[0].created, nil
}

// storeFingerprintOccurrences attaches events sharing one fingerprint to the
// incident last seen within the deduplication window, opening a new incident
// when none matches, and returns every incident it touched.
func storeFingerprintOccurrences(ctx context.Context, transaction pgx.Tx, ingestion config.IngestionConfig, fingerprint string, events []telemetry.Event) ([]*incidentOccurrences, error) {
	queries := sqlcstore.New(transaction)
	if lockError := queries.LockIncidentFingerprint(ctx, ingestion.ProjectID+":"+ingestion.EnvironmentID+":"+fingerprint); lockError != nil {
		return nil, fmt.Errorf("lock incident fingerprint: %w", lockError)
	}
	fresh, freshError := unrecordedEvents(ctx, queries, ingestion.ProjectID, events)
	if freshError != nil || len(fresh) == 0 {
		return nil, freshError
	}

	var touched []*incidentOccurrences
	byID := make(map[string]*incidentOccurrences)
	for _, event := range fresh {
		target := latestWithinWindow(touched, event.Timestamp)
		if target == nil {
			found, findError := findIncident(ctx, queries, ingestion, fingerprint, event.Timestamp)
			if findError != nil {
				return nil, findError
			}
			if found == nil {
				found = &incidentOccurrences{
					id:         telemetry.DeterministicID(event.Timestamp, "incident", ingestion.ProjectID, ingestion.EnvironmentID, fingerprint, event.ID),
					created:    true,
					lastSeenAt: event.Timestamp,
				}
			}
			if known, exists := byID[found.id]; exists {
				found = known
			} else {
				byID[found.id] = found
				touched = append(touched, found)
			}
			target = found
		}
		target.events = append(target.events, event)
		if event.Timestamp.After(target.lastSeenAt) {
			target.lastSeenAt = event.Timestamp
		}
	}

	// Every write goes out in one round trip.
	queue := &pgbatch.Queue{}
	writes := sqlcstore.New(queue)
	for _, record := range touched {
		queueIncidentWrite(ctx, writes, ingestion, fingerprint, record)
	}
	if sendError := queue.Send(ctx, transaction); sendError != nil {
		return nil, fmt.Errorf("write incident occurrences: %w", sendError)
	}
	return touched, nil
}

// unrecordedEvents drops duplicates and events a previous attempt already
// recorded, and returns the rest oldest first.
func unrecordedEvents(ctx context.Context, queries *sqlcstore.Queries, projectID string, events []telemetry.Event) ([]telemetry.Event, error) {
	unique := make(map[string]telemetry.Event, len(events))
	for _, event := range events {
		unique[event.ID] = event
	}
	recorded, queryError := queries.ListRecordedOccurrenceEventIDs(ctx, sqlcstore.ListRecordedOccurrenceEventIDsParams{
		ProjectID: projectID, EventIDs: slices.Collect(maps.Keys(unique)),
	})
	if queryError != nil {
		return nil, fmt.Errorf("check incident occurrences: %w", queryError)
	}
	for _, eventID := range recorded {
		delete(unique, eventID)
	}
	fresh := slices.Collect(maps.Values(unique))
	slices.SortFunc(fresh, func(first, second telemetry.Event) int {
		if order := first.Timestamp.Compare(second.Timestamp); order != 0 {
			return order
		}
		return strings.Compare(first.ID, second.ID)
	})
	return fresh, nil
}

// latestWithinWindow mirrors findIncident for incidents this batch already
// touched, whose last_seen_at in the database is not updated yet.
func latestWithinWindow(touched []*incidentOccurrences, at time.Time) *incidentOccurrences {
	var latest *incidentOccurrences
	for _, record := range touched {
		if record.lastSeenAt.Before(at.Add(-incidentDeduplicationWindow)) || record.lastSeenAt.After(at.Add(incidentDeduplicationWindow)) {
			continue
		}
		if latest == nil || record.lastSeenAt.After(latest.lastSeenAt) {
			latest = record
		}
	}
	return latest
}

func findIncident(ctx context.Context, queries *sqlcstore.Queries, ingestion config.IngestionConfig, fingerprint string, at time.Time) (*incidentOccurrences, error) {
	row, queryError := queries.FindIncidentInWindow(ctx, sqlcstore.FindIncidentInWindowParams{
		ProjectID: ingestion.ProjectID, EnvironmentID: ingestion.EnvironmentID, Fingerprint: fingerprint,
		WindowStart: at.Add(-incidentDeduplicationWindow), WindowEnd: at.Add(incidentDeduplicationWindow),
	})
	if errors.Is(queryError, pgx.ErrNoRows) {
		return nil, nil
	}
	if queryError != nil {
		return nil, fmt.Errorf("find incident: %w", queryError)
	}
	return &incidentOccurrences{id: row.ID, lastSeenAt: row.LastSeenAt}, nil
}
