package hybrid

import (
	"context"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/query"
)

// analysisWindow widens the RCA topology read when every evidence window ends together.
const analysisWindow = 5 * time.Minute

// Telemetry reads go to ClickHouse and never return data past the project's
// retention cutoff, even before the retention worker has removed it.

func (store *Store) ListTraces(ctx context.Context, project string, filter query.TraceFilter) ([]query.TraceSummary, error) {
	cutoff, err := store.retentionCutoff(ctx, project)
	if err != nil {
		return nil, err
	}
	filter.From = maxTime(filter.From, cutoff)
	return store.ch.ListTraces(ctx, project, filter)
}

func (store *Store) GetTrace(ctx context.Context, project, environment, traceID string, page query.SpanPage) ([]query.SpanRecord, bool, error) {
	cutoff, err := store.retentionCutoff(ctx, project)
	if err != nil {
		return nil, false, err
	}
	page.Since = cutoff
	return store.ch.GetTrace(ctx, project, environment, traceID, page)
}

func (store *Store) ListServices(ctx context.Context, project, environment string, from, to time.Time) ([]query.ServiceSummary, error) {
	cutoff, err := store.retentionCutoff(ctx, project)
	if err != nil {
		return nil, err
	}
	return store.ch.ListServices(ctx, project, environment, maxTime(from, cutoff), to)
}

func (store *Store) ListServiceDependencies(ctx context.Context, project, environment, service string) ([]query.ServiceDependency, error) {
	return store.ch.ListServiceDependencies(ctx, project, environment, service)
}

func (store *Store) ListTopology(ctx context.Context, project string, filter analysis.Filter) (analysis.Topology, error) {
	cutoff, err := store.retentionCutoff(ctx, project)
	if err != nil {
		return analysis.Topology{}, err
	}
	filter.From = maxTime(filter.From, cutoff)
	return store.ch.ListTopology(ctx, project, filter)
}

func (store *Store) ListMetricPoints(ctx context.Context, project string, filter analysis.Filter) ([]analysis.MetricPoint, error) {
	cutoff, err := store.retentionCutoff(ctx, project)
	if err != nil {
		return nil, err
	}
	filter.From = maxTime(filter.From, cutoff)
	return store.ch.ListMetricPoints(ctx, project, filter)
}

func (store *Store) TelemetryQuality(ctx context.Context, project string, filter analysis.Filter) (analysis.Quality, error) {
	cutoff, err := store.retentionCutoff(ctx, project)
	if err != nil {
		return analysis.Quality{}, err
	}
	quality := analysis.Quality{From: filter.From, To: filter.To}
	filter.From = maxTime(filter.From, cutoff)
	if quality.Spans, quality.OrphanSpans, quality.MetricPoints, err = store.ch.SignalQuality(ctx, project, filter); err != nil {
		return quality, err
	}
	quality.Logs, quality.UncorrelatedLogs, err = store.ch.LogQuality(ctx, project, filter.EnvironmentID, filter.Service, filter.From, filter.To)
	return quality, err
}

func (store *Store) RequestPerformance(ctx context.Context, project string, filter query.PerformanceFilter) (query.RequestPerformance, error) {
	cutoff, err := store.retentionCutoff(ctx, project)
	if err != nil {
		return query.RequestPerformance{}, err
	}
	filter.Since = cutoff
	return store.ch.RequestPerformance(ctx, project, filter)
}

// GetIncidentAnalysis ranks RCA hypotheses from the incident's latest evidence,
// stored in PostgreSQL, and the dependencies ClickHouse observed in its window.
// A missing incident yields an empty analysis.
func (store *Store) GetIncidentAnalysis(ctx context.Context, project, environment, incidentID string) (analysis.IncidentAnalysis, error) {
	service, evidence, found, err := store.IncidentEvidence(ctx, project, environment, incidentID)
	if err != nil || !found {
		return analysis.IncidentAnalysis{}, err
	}
	dependencies := []analysis.TopologyEdge{}
	if len(evidence) > 1 {
		from, to := analysis.EvidenceWindow(evidence, analysisWindow)
		topology, err := store.ListTopology(ctx, project, analysis.Filter{EnvironmentID: environment, From: from, To: to})
		if err != nil {
			return analysis.IncidentAnalysis{}, err
		}
		dependencies = topology.Edges
	}
	return analysis.BuildRCA(incidentID, service, evidence, dependencies), nil
}

func (store *Store) DetectServiceEvidence(ctx context.Context, scope config.IngestionConfig, end time.Time) ([]analysis.DetectedEvidence, error) {
	return store.ch.DetectServiceEvidence(ctx, scope, end)
}

func (store *Store) DetectDependencyEvidence(ctx context.Context, scope config.IngestionConfig, end time.Time) ([]analysis.DetectedEvidence, error) {
	return store.ch.DetectDependencyEvidence(ctx, scope, end)
}

func (store *Store) DetectMetricEvidence(ctx context.Context, scope config.IngestionConfig, end time.Time) ([]analysis.DetectedEvidence, error) {
	return store.ch.DetectMetricEvidence(ctx, scope, end)
}

// RefreshSpanRollups recomputes ClickHouse minute rollups that new or late spans made stale.
func (store *Store) RefreshSpanRollups(ctx context.Context) (int, error) {
	return store.ch.RefreshSpanRollups(ctx)
}

func maxTime(first, second time.Time) time.Time {
	if second.After(first) {
		return second
	}
	return first
}
