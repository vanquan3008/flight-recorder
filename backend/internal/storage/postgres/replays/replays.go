package replays

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	replaydomain "github.com/vanquan3008/tracepilot/internal/domain/replay"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

func (store *ReplayStore) FailStaleReplayExecutions(ctx context.Context, timeout time.Duration) (int64, error) {
	if timeout <= 0 {
		return 0, errors.New("replay timeout must be positive")
	}
	failed, executeError := store.queries.FailStaleReplayExecutions(ctx, time.Now().UTC().Add(-timeout))
	if executeError != nil {
		return 0, fmt.Errorf("fail stale replay executions: %w", executeError)
	}
	return failed, nil
}

// StartReplayExecution reports false when the incident is not in the caller's
// project and environment.
func (store *ReplayStore) StartReplayExecution(ctx context.Context, executionID, projectID, environmentID, incidentID, target, createdBy string) (bool, error) {
	started, executeError := store.queries.StartReplayExecution(ctx, sqlcstore.StartReplayExecutionParams{
		ID: executionID, ProjectID: projectID, EnvironmentID: environmentID,
		IncidentID: incidentID, Target: target, CreatedBy: createdBy,
	})
	if executeError != nil {
		return false, fmt.Errorf("start replay execution: %w", executeError)
	}
	return started == 1, nil
}

func (store *ReplayStore) CompleteReplayExecution(ctx context.Context, executionID, projectID string, result replaydomain.Result) error {
	comparison, marshalError := json.Marshal(map[string]any{
		"reason":         result.Reason,
		"content_type":   result.ContentType,
		"response_bytes": result.ResponseBytes,
		"body_truncated": result.BodyTruncated,
	})
	if marshalError != nil {
		return fmt.Errorf("marshal replay comparison: %w", marshalError)
	}
	completed, executeError := store.queries.CompleteReplayExecution(ctx, sqlcstore.CompleteReplayExecutionParams{
		ID: executionID, ProjectID: projectID, Target: result.Target, Status: result.Status,
		DurationNS: pgvalue.NanosOrNil(result.Duration), HTTPStatus: pgvalue.OrNil(int32(result.HTTPStatus)),
		Comparison: comparison,
	})
	if executeError != nil {
		return fmt.Errorf("complete replay execution: %w", executeError)
	}
	if completed != 1 {
		return errors.New("complete replay execution: execution not found")
	}
	return nil
}

func (store *ReplayStore) ListReplayExecutions(ctx context.Context, projectID, environmentID, incidentID string, filter replaydomain.ExecutionFilter) ([]replaydomain.Execution, error) {
	rows, queryError := store.queries.ListReplayExecutions(ctx, sqlcstore.ListReplayExecutionsParams{
		ProjectID: projectID, EnvironmentID: environmentID, IncidentID: incidentID,
		Before:   pgvalue.TimeOrNil(filter.Before),
		CursorID: pgvalue.OrNil(filter.CursorID),
		RowLimit: int32(filter.Limit + 1),
	})
	if queryError != nil {
		return nil, fmt.Errorf("list replay executions: %w", queryError)
	}
	executions := make([]replaydomain.Execution, 0, len(rows))
	for _, row := range rows {
		executions = append(executions, execution(sqlcstore.GetReplayExecutionRow(row)))
	}
	return executions, nil
}

// GetReplayExecution returns a zero Execution when it does not exist in the
// caller's project and environment.
func (store *ReplayStore) GetReplayExecution(ctx context.Context, projectID, environmentID, incidentID, executionID string) (replaydomain.Execution, error) {
	row, queryError := store.queries.GetReplayExecution(ctx, sqlcstore.GetReplayExecutionParams{
		ProjectID: projectID, EnvironmentID: environmentID, IncidentID: incidentID, ID: executionID,
	})
	if errors.Is(queryError, pgx.ErrNoRows) {
		return replaydomain.Execution{}, nil
	}
	if queryError != nil {
		return replaydomain.Execution{}, fmt.Errorf("get replay execution: %w", queryError)
	}
	return execution(row), nil
}

func execution(row sqlcstore.GetReplayExecutionRow) replaydomain.Execution {
	return replaydomain.Execution{
		ID: row.ID, IncidentID: row.IncidentID, Target: row.Target, Status: row.Status,
		StartedAt: row.StartedAt, CompletedAt: row.CompletedAt, DurationNS: row.DurationNS,
		HTTPStatus: pgvalue.IntPointer(row.HTTPStatus), Comparison: row.Comparison,
		CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt,
	}
}
