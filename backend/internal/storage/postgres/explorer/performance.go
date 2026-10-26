package explorer

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

func (store *QueryStore) RequestPerformance(ctx context.Context, projectID string, filter query.PerformanceFilter) (query.RequestPerformance, error) {
	if err := filter.Validate(); err != nil {
		return query.RequestPerformance{}, err
	}
	result := newRequestPerformance(filter)
	rows, err := store.queries.RequestPerformance(ctx, sqlcstore.RequestPerformanceParams{
		ProjectID: projectID, EnvironmentID: filter.EnvironmentID,
		FromTime: filter.From, ToTime: filter.To,
		Service: pgvalue.OrNil(filter.Service), Endpoint: pgvalue.OrNil(filter.Endpoint),
		BucketSeconds: int32(result.BucketSeconds), RowLimit: int32(filter.Limit),
	})
	if err != nil {
		return result, fmt.Errorf("query request performance: %w", err)
	}
	for _, row := range rows {
		stats := query.PerformanceStats{
			Requests: row.Requests, Errors: row.Errors, AverageLatencyNanos: row.AverageNs,
			P95LatencyNanos: row.P95Ns, P99LatencyNanos: row.P99Ns,
		}
		stats.SetRates(filter.To.Sub(filter.From).Seconds())
		switch row.Dimension {
		case "summary":
			result.Summary = stats
		case "service":
			result.Services = append(result.Services, query.PerformanceGroup{Service: row.Service, PerformanceStats: stats})
		case "endpoint":
			result.Endpoints = append(result.Endpoints, query.PerformanceGroup{Service: row.Service, Endpoint: row.Endpoint, PerformanceStats: stats})
		case "bucket":
			if row.Bucket < 0 || int(row.Bucket) >= len(result.Series) {
				return result, fmt.Errorf("request performance bucket %d outside window", row.Bucket)
			}
			interval := &result.Series[row.Bucket]
			stats.SetRates(interval.To.Sub(interval.From).Seconds())
			interval.PerformanceStats = stats
		}
	}
	return result, nil
}

func newRequestPerformance(filter query.PerformanceFilter) query.RequestPerformance {
	seconds := max(1, int(math.Ceil(filter.To.Sub(filter.From).Seconds()/60)))
	result := query.RequestPerformance{
		From: filter.From.UTC(), To: filter.To.UTC(), Service: filter.Service, Endpoint: filter.Endpoint,
		BucketSeconds: seconds, Limit: filter.Limit,
		Series: make([]query.PerformanceBucket, 0, 60), Services: []query.PerformanceGroup{}, Endpoints: []query.PerformanceGroup{},
	}
	for from := result.From; from.Before(result.To); from = from.Add(time.Duration(seconds) * time.Second) {
		to := from.Add(time.Duration(seconds) * time.Second)
		if to.After(result.To) {
			to = result.To
		}
		result.Series = append(result.Series, query.PerformanceBucket{From: from, To: to})
	}
	return result
}
