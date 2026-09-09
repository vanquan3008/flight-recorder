package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/security"
)

func (database *Database) ListSecurityDetections(ctx context.Context, projectID, environmentID string, limit int, before time.Time, cursorID string) ([]security.Detection, error) {
	rows, queryError := database.pool.Query(ctx, `SELECT id, rule_id, rule_version, security_event_id, COALESCE(incident_id::text, ''), group_key, window_started_at, window_ended_at, severity, created_at FROM security_detections WHERE project_id=$1 AND environment_id=$2 AND ($3::timestamptz IS NULL OR created_at < $3 OR (created_at=$3 AND id < $4::uuid)) ORDER BY created_at DESC, id DESC LIMIT $5`, projectID, environmentID, nullableTime(before), nullableString(cursorID), limit+1)
	if queryError != nil {
		return nil, fmt.Errorf("list security detections: %w", queryError)
	}
	defer rows.Close()
	detections := make([]security.Detection, 0, limit+1)
	for rows.Next() {
		var item security.Detection
		if scanError := rows.Scan(&item.ID, &item.RuleID, &item.RuleVersion, &item.SecurityEventID, &item.IncidentID, &item.GroupKey, &item.WindowStartedAt, &item.WindowEndedAt, &item.Severity, &item.CreatedAt); scanError != nil {
			return nil, fmt.Errorf("scan security detection: %w", scanError)
		}
		detections = append(detections, item)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate security detections: %w", rowsError)
	}
	return detections, nil
}
