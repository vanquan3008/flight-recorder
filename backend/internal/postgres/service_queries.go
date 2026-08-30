package postgres

import (
	"context"
	"fmt"
	"time"
)

func (database *Database) ListServices(ctx context.Context, projectID, environmentID string, from, to time.Time) ([]ServiceSummary, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT service_name, count(DISTINCT trace_id), count(*) FILTER (WHERE span_kind = 2),
			count(*) FILTER (WHERE status_code = 'ERROR'),
			COALESCE(count(*) FILTER (WHERE status_code = 'ERROR')::double precision / NULLIF(count(*), 0), 0),
			COALESCE(avg(extract(epoch FROM ended_at - started_at) * 1000000000)::bigint, 0)
		FROM spans WHERE project_id = $1 AND environment_id = $2 AND ($3::timestamptz IS NULL OR started_at >= $3)
			AND ($4::timestamptz IS NULL OR started_at <= $4) GROUP BY service_name ORDER BY service_name`,
		projectID, environmentID, nullableTime(from), nullableTime(to))
	if queryError != nil {
		return nil, fmt.Errorf("list services: %w", queryError)
	}
	defer rows.Close()
	services := make([]ServiceSummary, 0)
	for rows.Next() {
		var service ServiceSummary
		if scanError := rows.Scan(&service.Name, &service.TraceCount, &service.RequestCount, &service.ErrorCount, &service.ErrorRate, &service.AverageLatency); scanError != nil {
			return nil, fmt.Errorf("scan service: %w", scanError)
		}
		services = append(services, service)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate services: %w", rowsError)
	}
	return services, nil
}

func (database *Database) ListServiceDependencies(ctx context.Context, projectID, environmentID, serviceName string) ([]ServiceDependency, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT child.service_name, count(*) FROM spans child
		JOIN spans parent ON parent.project_id = child.project_id AND parent.trace_id = child.trace_id AND parent.span_id = child.parent_span_id
		WHERE child.project_id = $1 AND child.environment_id = $2 AND parent.environment_id = $2
			AND parent.service_name = $3 AND parent.service_name <> child.service_name
		GROUP BY child.service_name ORDER BY count(*) DESC, child.service_name`, projectID, environmentID, serviceName)
	if queryError != nil {
		return nil, fmt.Errorf("list service dependencies: %w", queryError)
	}
	defer rows.Close()
	dependencies := make([]ServiceDependency, 0)
	for rows.Next() {
		var dependency ServiceDependency
		if scanError := rows.Scan(&dependency.ServiceName, &dependency.CallCount); scanError != nil {
			return nil, fmt.Errorf("scan service dependency: %w", scanError)
		}
		dependencies = append(dependencies, dependency)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate service dependencies: %w", rowsError)
	}
	return dependencies, nil
}
