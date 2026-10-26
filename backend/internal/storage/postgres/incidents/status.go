package incidents

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/outbox"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// UpdateIncidentStatus applies an allowed transition, records who made it and
// queues the matching webhook in the same transaction.
func (store *IncidentStore) UpdateIncidentStatus(ctx context.Context, projectID, environmentID, incidentID, requestedStatus, actorKeyID string) (incident.Incident, error) {
	status, valid := incident.NormalizeStatus(requestedStatus)
	if !valid || actorKeyID == "" {
		return incident.Incident{}, incident.ErrInvalidTransition
	}
	updateError := pgx.BeginFunc(ctx, store.pool, func(transaction pgx.Tx) error {
		queries := store.queries.WithTx(transaction)
		scope := sqlcstore.LockIncidentStatusParams{ProjectID: projectID, EnvironmentID: environmentID, ID: incidentID}
		previous, lockError := queries.LockIncidentStatus(ctx, scope)
		if errors.Is(lockError, pgx.ErrNoRows) {
			return incident.ErrNotFound
		}
		if lockError != nil {
			return fmt.Errorf("load incident status: %w", lockError)
		}
		if !incident.CanTransition(previous, status) {
			return incident.ErrInvalidTransition
		}

		changedAt := time.Now().UTC()
		if setError := queries.SetIncidentStatus(ctx, sqlcstore.SetIncidentStatusParams{
			ProjectID: projectID, EnvironmentID: environmentID, ID: incidentID, Status: status, ChangedAt: changedAt,
		}); setError != nil {
			return fmt.Errorf("set incident status: %w", setError)
		}
		if insertError := queries.InsertIncidentStatusChange(ctx, sqlcstore.InsertIncidentStatusChangeParams{
			ID:        telemetry.DeterministicID(changedAt, "incident-status", projectID, incidentID, previous, status, actorKeyID),
			ProjectID: projectID, IncidentID: incidentID, PreviousStatus: previous, Status: status,
			ActorKeyID: actorKeyID, ChangedAt: changedAt,
		}); insertError != nil {
			return fmt.Errorf("record incident status change: %w", insertError)
		}
		eventType, notifies := statusEvents[status]
		if !notifies {
			return nil
		}
		return outbox.EnqueueDeliveries(ctx, transaction, projectID, eventType, map[string]any{
			"incident_id": incidentID, "environment_id": environmentID,
			"previous_status": previous, "status": status, "actor_key_id": actorKeyID,
		})
	})
	if updateError != nil {
		return incident.Incident{}, fmt.Errorf("update incident status: %w", updateError)
	}
	return store.GetIncident(ctx, projectID, environmentID, incidentID)
}

// statusEvents maps the statuses that notify subscribers to their webhook event.
var statusEvents = map[string]string{
	"OPEN":     "incident.opened",
	"RESOLVED": "incident.resolved",
}
