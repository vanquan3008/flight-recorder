package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/flight-observability/flight/internal/replay"
)

func (database *Database) StartReplayExecution(ctx context.Context, executionID, projectID, incidentID, target, createdBy string) (bool, error) {
	command, executeError := database.pool.Exec(ctx, `
		INSERT INTO replay_executions (id, project_id, incident_id, target, status, started_at, created_by)
		SELECT $1, $2, id, $3, 'started', now(), $4
		FROM incidents
		WHERE project_id = $2 AND id = $5`, executionID, projectID, target, createdBy, incidentID)
	if executeError != nil {
		return false, fmt.Errorf("start replay execution: %w", executeError)
	}
	return command.RowsAffected() == 1, nil
}

func (database *Database) CompleteReplayExecution(ctx context.Context, executionID, projectID string, result replay.Result) error {
	comparison, marshalError := json.Marshal(map[string]any{
		"reason":         result.Reason,
		"content_type":   result.ContentType,
		"response_bytes": result.ResponseBytes,
		"body_truncated": result.BodyTruncated,
	})
	if marshalError != nil {
		return fmt.Errorf("marshal replay comparison: %w", marshalError)
	}
	command, executeError := database.pool.Exec(ctx, `
		UPDATE replay_executions
		SET target = $3, status = $4, completed_at = now(), duration_ns = $5,
			http_status = $6, comparison = $7
		WHERE id = $1 AND project_id = $2 AND status = 'started'`,
		executionID, projectID, result.Target, result.Status, nullableDuration(result.Duration), nullableInt(result.HTTPStatus), comparison)
	if executeError != nil {
		return fmt.Errorf("complete replay execution: %w", executeError)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("complete replay execution: execution not found")
	}
	return nil
}
