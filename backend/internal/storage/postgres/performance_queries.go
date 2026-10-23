package postgres

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/vanquan3008/tracepilot/internal/query"
)

// One statement keeps summary, percentiles and breakdowns on the same snapshot.
const requestPerformanceSQL = `
	WITH inbound AS (
		SELECT service_name, started_at,
			CASE
				WHEN NULLIF(span_attributes->>'http.route','') IS NOT NULL THEN
					concat_ws(' ', COALESCE(NULLIF(span_attributes->>'http.request.method',''),
						NULLIF(span_attributes->>'http.method','')), span_attributes->>'http.route')
				WHEN NULLIF(span_attributes->>'rpc.method','') IS NOT NULL THEN
					concat_ws('/', NULLIF(span_attributes->>'rpc.service',''), span_attributes->>'rpc.method')
				ELSE COALESCE(NULLIF(span_name,''), '(unnamed)')
			END endpoint,
			extract(epoch FROM ended_at-started_at)*1000000000 duration_ns,
			(status_code='ERROR' OR COALESCE(NULLIF(span_attributes->>'http.response.status_code',''),
				span_attributes->>'http.status_code','') ~ '^5[0-9]{2}$') failed
		FROM spans
		WHERE project_id=$1 AND environment_id=$2 AND span_kind=2
			AND started_at >= $3 AND started_at < $4 AND ($5='' OR service_name=$5)
	), observed AS (
		SELECT *, floor(extract(epoch FROM started_at-$3::timestamptz)/$7)::int bucket
		FROM inbound WHERE $6='' OR endpoint=$6
	), grouped AS (
		SELECT CASE WHEN grouping(bucket)=0 THEN 'bucket'
			WHEN grouping(endpoint)=0 THEN 'endpoint'
			WHEN grouping(service_name)=0 THEN 'service' ELSE 'summary' END dimension,
			COALESCE(service_name,'') service, COALESCE(endpoint,'') endpoint,
			COALESCE(bucket,-1) bucket, count(*) requests, count(*) FILTER (WHERE failed) errors,
			COALESCE(avg(duration_ns),0)::bigint average_ns,
			COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ns),0)::bigint p95_ns,
			COALESCE(percentile_cont(0.99) WITHIN GROUP (ORDER BY duration_ns),0)::bigint p99_ns
		FROM observed GROUP BY GROUPING SETS ((), (service_name), (service_name,endpoint), (bucket))
	), ranked AS (
		SELECT *, row_number() OVER (PARTITION BY dimension ORDER BY requests DESC,service,endpoint) position
		FROM grouped
	)
	SELECT dimension,service,endpoint,bucket,requests,errors,average_ns,p95_ns,p99_ns
	FROM ranked WHERE dimension IN ('summary','bucket') OR position <= $8
	ORDER BY dimension,position`

func (database *Database) RequestPerformance(ctx context.Context, projectID string, filter query.PerformanceFilter) (query.RequestPerformance, error) {
	if err := filter.Validate(); err != nil {
		return query.RequestPerformance{}, err
	}
	result := newRequestPerformance(filter)
	rows, err := database.pool.Query(ctx, requestPerformanceSQL, projectID, filter.EnvironmentID,
		filter.From, filter.To, filter.Service, filter.Endpoint, result.BucketSeconds, filter.Limit)
	if err != nil {
		return result, fmt.Errorf("query request performance: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var dimension, service, endpoint string
		var bucket int
		var stats query.PerformanceStats
		if err := rows.Scan(&dimension, &service, &endpoint, &bucket, &stats.Requests, &stats.Errors,
			&stats.AverageLatencyNanos, &stats.P95LatencyNanos, &stats.P99LatencyNanos); err != nil {
			return result, fmt.Errorf("scan request performance: %w", err)
		}
		stats.SetRates(filter.To.Sub(filter.From).Seconds())
		switch dimension {
		case "summary":
			result.Summary = stats
		case "service":
			result.Services = append(result.Services, query.PerformanceGroup{Service: service, PerformanceStats: stats})
		case "endpoint":
			result.Endpoints = append(result.Endpoints, query.PerformanceGroup{Service: service, Endpoint: endpoint, PerformanceStats: stats})
		case "bucket":
			if bucket < 0 || bucket >= len(result.Series) {
				return result, fmt.Errorf("request performance bucket %d outside window", bucket)
			}
			interval := &result.Series[bucket]
			stats.SetRates(interval.To.Sub(interval.From).Seconds())
			interval.PerformanceStats = stats
		}
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterate request performance: %w", err)
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
