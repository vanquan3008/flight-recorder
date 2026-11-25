package incidents

import (
	"cmp"
	"context"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// incidentDetails holds the optional incident columns. Each keeps the newest
// non-empty value across occurrences.
type incidentDetails struct {
	traceID      string
	requestID    string
	errorType    string
	errorMessage string
	httpStatus   int
}

func newestDetails(events []telemetry.Event) incidentDetails {
	var details incidentDetails
	for _, event := range events { // oldest first, so later values win
		details.traceID = cmp.Or(event.TraceID, details.traceID)
		details.requestID = cmp.Or(event.RequestID, details.requestID)
		details.errorType = cmp.Or(event.ErrorType, details.errorType)
		details.errorMessage = cmp.Or(event.ErrorMessage, details.errorMessage)
		details.httpStatus = cmp.Or(event.HTTPStatus, details.httpStatus)
	}
	return details
}

// queueIncidentWrite queues the incident row once, then one occurrence, event
// link and evidence row per event. writes must be backed by a pgbatch.Queue,
// whose Send reports execution errors.
func queueIncidentWrite(ctx context.Context, writes *sqlcstore.Queries, ingestion config.IngestionConfig, fingerprint string, record *incidentOccurrences) {
	first, latest := record.events[0], record.events[len(record.events)-1]
	details := newestDetails(record.events)
	if record.created {
		writes.InsertIncident(ctx, sqlcstore.InsertIncidentParams{
			ID: record.id, ProjectID: ingestion.ProjectID, EnvironmentID: ingestion.EnvironmentID,
			PrimaryService: first.ServiceName, Fingerprint: fingerprint, Title: incident.Title(first),
			TraceID: pgvalue.OrNil(details.traceID), RequestID: pgvalue.OrNil(details.requestID),
			ErrorType: pgvalue.OrNil(details.errorType), ErrorMessage: pgvalue.OrNil(details.errorMessage),
			HTTPStatus: pgvalue.OrNil(int32(details.httpStatus)), Severity: incident.Severity(latest),
			FirstSeenAt: first.Timestamp, LastSeenAt: latest.Timestamp, OccurrenceCount: int64(len(record.events)),
		})
	} else {
		writes.AddIncidentOccurrences(ctx, sqlcstore.AddIncidentOccurrencesParams{
			ProjectID: ingestion.ProjectID, ID: record.id,
			FirstSeenAt: first.Timestamp, LastSeenAt: latest.Timestamp, OccurrenceCount: int64(len(record.events)),
			TraceID: pgvalue.OrNil(details.traceID), RequestID: pgvalue.OrNil(details.requestID),
			ErrorType: pgvalue.OrNil(details.errorType), ErrorMessage: pgvalue.OrNil(details.errorMessage),
			HTTPStatus: pgvalue.OrNil(int32(details.httpStatus)), Severity: incident.Severity(latest),
		})
	}
	for _, event := range record.events {
		writes.InsertIncidentOccurrence(ctx, sqlcstore.InsertIncidentOccurrenceParams{
			ID:        telemetry.DeterministicID(event.Timestamp, "incident-occurrence", record.id, event.ID),
			ProjectID: ingestion.ProjectID, IncidentID: record.id, EventID: event.ID, OccurredAt: event.Timestamp,
		})
		writes.LinkIncidentEvent(ctx, sqlcstore.LinkIncidentEventParams{ProjectID: ingestion.ProjectID, IncidentID: record.id, EventID: event.ID})
		queueEventEvidence(ctx, writes, ingestion, record.id, event)
	}
}

func queueEventEvidence(ctx context.Context, writes *sqlcstore.Queries, ingestion config.IngestionConfig, incidentID string, event telemetry.Event) {
	// A detector's summary is derived evidence, not another measured error.
	if event.Signal == "" && event.Type == telemetry.EventCustom && event.ErrorType == "OBSERVABILITY_ANOMALY" && event.Attributes["detector"] == "baseline" {
		return
	}
	traceIDs := []string{}
	if event.TraceID != "" {
		traceIDs = append(traceIDs, event.TraceID)
	}
	writes.InsertEventEvidence(ctx, sqlcstore.InsertEventEvidenceParams{
		ID:        telemetry.DeterministicID(event.Timestamp, "incident-event-evidence", incidentID, event.ID),
		ProjectID: ingestion.ProjectID, EnvironmentID: ingestion.EnvironmentID, IncidentID: incidentID,
		EntityID: analysis.EntityID("service", event.ServiceName), OccurredAt: event.Timestamp,
		TraceIDs: traceIDs, Summary: cmp.Or(event.ErrorMessage, incident.Title(event)),
	})
}
