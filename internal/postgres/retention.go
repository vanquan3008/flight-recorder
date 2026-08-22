package postgres

import (
	"context"
	"fmt"
	"time"
)

func (database *Database) DeleteEventsBefore(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	result, deleteError := database.pool.Exec(ctx, `
		WITH expired AS (
			SELECT id
			FROM events
			WHERE occurred_at < $1
			ORDER BY occurred_at, id
			LIMIT $2
		)
		DELETE FROM events
		USING expired
		WHERE events.id = expired.id`, before, batchSize)
	if deleteError != nil {
		return 0, fmt.Errorf("delete expired events: %w", deleteError)
	}
	return result.RowsAffected(), nil
}
