package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/domain/security"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func securitySourceType(event telemetry.Event) string {
	if value, ok := event.Attributes["log.source.type"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return "unknown"
}

func (database *Database) ListSecuritySources(ctx context.Context, projectID, environmentID string, limit int) ([]security.Source, error) {
	rows, queryError := database.pool.Query(ctx, `SELECT id, name, source_type, format, last_seen_at, accepted_count, error_count, created_at FROM security_sources WHERE project_id=$1 AND environment_id=$2 ORDER BY last_seen_at DESC, id DESC LIMIT $3`, projectID, environmentID, limit)
	if queryError != nil {
		return nil, fmt.Errorf("list security sources: %w", queryError)
	}
	defer rows.Close()
	sources := make([]security.Source, 0, limit)
	for rows.Next() {
		var source security.Source
		if scanError := rows.Scan(&source.ID, &source.Name, &source.Type, &source.Format, &source.LastSeenAt, &source.AcceptedCount, &source.ErrorCount, &source.CreatedAt); scanError != nil {
			return nil, fmt.Errorf("scan security source: %w", scanError)
		}
		sources = append(sources, source)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate security sources: %w", rowsError)
	}
	return sources, nil
}
