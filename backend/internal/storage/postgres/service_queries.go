package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/vanquan3008/tracepilot/internal/query"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

func (database *Database) ListServices(ctx context.Context, projectID, environmentID string, from, to time.Time) ([]query.ServiceSummary, error) {
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
	services := make([]query.ServiceSummary, 0)
	for _, row := range rows {
		services = append(services, query.ServiceSummary{
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

func (database *Database) ListServiceDependencies(ctx context.Context, projectID, environmentID, serviceName string) ([]query.ServiceDependency, error) {
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
	dependencies := make([]query.ServiceDependency, 0)
	for _, row := range rows {
		dependencies = append(dependencies, query.ServiceDependency{
			ServiceName: row.ServiceName,
			CallCount:   row.CallCount,
		})
	}
	return dependencies, nil
}
