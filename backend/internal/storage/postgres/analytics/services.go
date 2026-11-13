package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

func (store *AnalyticsStore) ListServices(ctx context.Context, projectID, environmentID string, from, to time.Time) ([]query.ServiceSummary, error) {
	rows, queryError := store.queries.ListServices(ctx, sqlcstore.ListServicesParams{
		ProjectID:     projectID,
		EnvironmentID: environmentID,
		FromTime:      pgvalue.TimeOrNil(from),
		ToTime:        pgvalue.TimeOrNil(to),
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

func (store *AnalyticsStore) ListServiceDependencies(ctx context.Context, projectID, environmentID, serviceName string) ([]query.ServiceDependency, error) {
	rows, queryError := store.queries.ListServiceDependencies(ctx, sqlcstore.ListServiceDependenciesParams{
		ProjectID:     projectID,
		EnvironmentID: environmentID,
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
