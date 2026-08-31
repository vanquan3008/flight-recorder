package postgres

import (
	"context"
	"fmt"
	"time"

	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

func (database *Database) ListServices(ctx context.Context, projectID, environmentID string, from, to time.Time) ([]ServiceSummary, error) {
	project, environment, scopeError := sqlcScope(projectID, environmentID)
	if scopeError != nil {
		return nil, fmt.Errorf("list services: %w", scopeError)
	}
	rows, queryError := database.queries.ListServices(ctx, sqlcstore.ListServicesParams{
		ProjectID:     project,
		EnvironmentID: environment,
		HasFrom:       !from.IsZero(),
		FromTime:      sqlcTime(from),
		HasTo:         !to.IsZero(),
		ToTime:        sqlcTime(to),
	})
	if queryError != nil {
		return nil, fmt.Errorf("list services: %w", queryError)
	}
	services := make([]ServiceSummary, 0)
	for _, row := range rows {
		services = append(services, ServiceSummary{
			Name:           row.Name,
			TraceCount:     row.TraceCount,
			RequestCount:   row.RequestCount,
			ErrorCount:     row.ErrorCount,
			ErrorRate:      row.ErrorRate,
			AverageLatency: row.AverageLatency,
		})
	}
	return services, nil
}

func (database *Database) ListServiceDependencies(ctx context.Context, projectID, environmentID, serviceName string) ([]ServiceDependency, error) {
	project, environment, scopeError := sqlcScope(projectID, environmentID)
	if scopeError != nil {
		return nil, fmt.Errorf("list service dependencies: %w", scopeError)
	}
	rows, queryError := database.queries.ListServiceDependencies(ctx, sqlcstore.ListServiceDependenciesParams{
		ProjectID:     project,
		EnvironmentID: environment,
		ServiceName:   serviceName,
	})
	if queryError != nil {
		return nil, fmt.Errorf("list service dependencies: %w", queryError)
	}
	dependencies := make([]ServiceDependency, 0)
	for _, row := range rows {
		dependencies = append(dependencies, ServiceDependency{
			ServiceName: row.ServiceName,
			CallCount:   row.CallCount,
		})
	}
	return dependencies, nil
}
