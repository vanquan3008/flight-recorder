package identity

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/auth"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

func (store *IdentityStore) GetProjectRetention(ctx context.Context, projectID string) (*int32, error) {
	days, err := store.queries.GetProjectRetention(ctx, projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, auth.ErrProjectNotFound
	}
	return days, err
}

func (store *IdentityStore) UpdateProjectRetention(ctx context.Context, projectID string, days *int32, actor auth.Actor) error {
	return store.inTransaction(ctx, func(queries *sqlcstore.Queries) error {
		_, err := queries.UpdateProjectRetention(ctx, sqlcstore.UpdateProjectRetentionParams{ProjectID: projectID, RetentionDays: days})
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.ErrProjectNotFound
		}
		if err != nil {
			return err
		}
		return audit(ctx, queries, projectID, actor, "project.retention_updated", "project", projectID, map[string]any{"retention_days": days})
	})
}
