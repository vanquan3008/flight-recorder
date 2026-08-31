package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	replaydomain "github.com/vanquan3008/tracepilot/internal/domain/replay"
)

func (database *Database) FailStaleReplayExecutions(ctx context.Context, timeout time.Duration) (int64, error) {
	if timeout <= 0 {
		return 0, errors.New("replay timeout must be positive")
	}
	result, executeError := database.pool.Exec(ctx, `
		UPDATE replay_executions
		SET status = 'failed', completed_at = now(),
			comparison = '{"reason":"replay execution timed out before completion"}'::jsonb
		WHERE status = 'started' AND started_at < $1`, time.Now().UTC().Add(-timeout))
	if executeError != nil {
		return 0, fmt.Errorf("fail stale replay executions: %w", executeError)
	}
	return result.RowsAffected(), nil
}

func (database *Database) StartReplayExecution(ctx context.Context, executionID, projectID, environmentID, incidentID, target, createdBy string) (bool, error) {
	command, executeError := database.pool.Exec(ctx, `
		INSERT INTO replay_executions (id, project_id, incident_id, target, status, started_at, created_by)
		SELECT $1, $2, id, $4, 'started', now(), $5
		FROM incidents
		WHERE project_id = $2 AND environment_id = $3 AND id = $6`, executionID, projectID, environmentID, target, createdBy, incidentID)
	if executeError != nil {
		return false, fmt.Errorf("start replay execution: %w", executeError)
	}
	return command.RowsAffected() == 1, nil
}

func (database *Database) CompleteReplayExecution(ctx context.Context, executionID, projectID string, result replaydomain.Result) error {
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

func (database *Database) ListReplayExecutions(ctx context.Context, projectID, environmentID, incidentID string, filter replaydomain.ExecutionFilter) ([]replaydomain.Execution, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT id, incident_id, target, status, started_at, completed_at, duration_ns, http_status,
			COALESCE(comparison, '{}'::jsonb), created_by, created_at
		FROM replay_executions
		WHERE project_id = $1 AND incident_id = $2
			AND EXISTS (SELECT 1 FROM incidents WHERE project_id = $1 AND environment_id = $3 AND id = replay_executions.incident_id)
			AND ($4::timestamptz IS NULL OR created_at < $4 OR (created_at = $4 AND id < $5::uuid))
		ORDER BY created_at DESC, id DESC
		LIMIT $6`, projectID, incidentID, environmentID, nullableTime(filter.Before), nullableString(filter.CursorID), filter.Limit+1)
	if queryError != nil {
		return nil, fmt.Errorf("list allowed request executions: %w", queryError)
	}
	defer rows.Close()
	executions := make([]replaydomain.Execution, 0, filter.Limit+1)
	for rows.Next() {
		var execution replaydomain.Execution
		if scanError := rows.Scan(
			&execution.ID, &execution.IncidentID, &execution.Target, &execution.Status,
			&execution.StartedAt, &execution.CompletedAt, &execution.DurationNS, &execution.HTTPStatus,
			&execution.Comparison, &execution.CreatedBy, &execution.CreatedAt,
		); scanError != nil {
			return nil, fmt.Errorf("scan allowed request execution: %w", scanError)
		}
		executions = append(executions, execution)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate allowed request executions: %w", rowsError)
	}
	return executions, nil
}

func (database *Database) GetReplayExecution(ctx context.Context, projectID, environmentID, incidentID, executionID string) (replaydomain.Execution, error) {
	var execution replaydomain.Execution
	queryError := database.pool.QueryRow(ctx, `
		SELECT id, incident_id, target, status, started_at, completed_at, duration_ns, http_status,
			COALESCE(comparison, '{}'::jsonb), created_by, created_at
		FROM replay_executions
		WHERE project_id = $1 AND incident_id = $2 AND id = $3
			AND EXISTS (SELECT 1 FROM incidents WHERE project_id = $1 AND environment_id = $4 AND id = replay_executions.incident_id)`, projectID, incidentID, executionID, environmentID).
		Scan(
			&execution.ID, &execution.IncidentID, &execution.Target, &execution.Status,
			&execution.StartedAt, &execution.CompletedAt, &execution.DurationNS, &execution.HTTPStatus,
			&execution.Comparison, &execution.CreatedBy, &execution.CreatedAt,
		)
	if queryError != nil {
		if errors.Is(queryError, pgx.ErrNoRows) {
			return replaydomain.Execution{}, nil
		}
		return replaydomain.Execution{}, fmt.Errorf("get allowed request execution: %w", queryError)
	}
	return execution, nil
}
