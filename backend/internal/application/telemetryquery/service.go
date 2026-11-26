// Package telemetryquery applies project retention to telemetry reads.
package telemetryquery

import (
	"context"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/query"
)

type logReader interface {
	ListLogs(context.Context, string, query.LogFilter) ([]query.LogRecord, error)
	GetLogSince(context.Context, string, string, string, time.Time) (query.LogRecord, error)
	LogQuality(context.Context, string, string, string, time.Time, time.Time) (int64, int64, error)
}

type traceReader interface {
	ListTraces(context.Context, string, query.TraceFilter) ([]query.TraceSummary, error)
	GetTrace(context.Context, string, string, string, query.SpanPage) ([]query.SpanRecord, bool, error)
	ListServices(context.Context, string, string, time.Time, time.Time) ([]query.ServiceSummary, error)
	ListServiceDependencies(context.Context, string, string, string, time.Time) ([]query.ServiceDependency, error)
}

type analyticsReader interface {
	ListTopology(context.Context, string, analysis.Filter) (analysis.Topology, error)
	ListMetricPoints(context.Context, string, analysis.Filter) ([]analysis.MetricPoint, error)
	SignalQuality(context.Context, string, analysis.Filter) (int64, int64, int64, error)
	RequestPerformance(context.Context, string, query.PerformanceFilter) (query.RequestPerformance, error)
}

type incidentEvidenceReader interface {
	IncidentEvidence(context.Context, string, string, string) (string, []analysis.Evidence, bool, error)
}

type cutoffReader interface {
	Cutoff(context.Context, string) (time.Time, error)
}

type Service struct {
	logs      logReader
	traces    traceReader
	analytics analyticsReader
	incidents incidentEvidenceReader
	retention cutoffReader
}

func New(logs logReader, traces traceReader, analytics analyticsReader, incidents incidentEvidenceReader, retention cutoffReader) *Service {
	return &Service{logs: logs, traces: traces, analytics: analytics, incidents: incidents, retention: retention}
}

func later(first, second time.Time) time.Time {
	if second.After(first) {
		return second
	}
	return first
}

// analysisWindow widens the RCA topology read when every evidence window ends together.
const analysisWindow = 5 * time.Minute

func (service *Service) ListLogs(ctx context.Context, project string, filter query.LogFilter) ([]query.LogRecord, error) {
	cutoff, err := service.retention.Cutoff(ctx, project)
	if err != nil {
		return nil, err
	}
	filter.From = later(filter.From, cutoff)
	return service.logs.ListLogs(ctx, project, filter)
}

func (service *Service) GetLog(ctx context.Context, project, environment, id string) (query.LogRecord, error) {
	cutoff, err := service.retention.Cutoff(ctx, project)
	if err != nil {
		return query.LogRecord{}, err
	}
	return service.logs.GetLogSince(ctx, project, environment, id, cutoff)
}

func (service *Service) ListTraces(ctx context.Context, project string, filter query.TraceFilter) ([]query.TraceSummary, error) {
	cutoff, err := service.retention.Cutoff(ctx, project)
	if err != nil {
		return nil, err
	}
	filter.Since = cutoff
	return service.traces.ListTraces(ctx, project, filter)
}

func (service *Service) GetTrace(ctx context.Context, project, environment, traceID string, page query.SpanPage) ([]query.SpanRecord, bool, error) {
	cutoff, err := service.retention.Cutoff(ctx, project)
	if err != nil {
		return nil, false, err
	}
	page.Since = cutoff
	return service.traces.GetTrace(ctx, project, environment, traceID, page)
}

func (service *Service) ListServices(ctx context.Context, project, environment string, from, to time.Time) ([]query.ServiceSummary, error) {
	cutoff, err := service.retention.Cutoff(ctx, project)
	if err != nil {
		return nil, err
	}
	return service.traces.ListServices(ctx, project, environment, later(from, cutoff), to)
}

func (service *Service) ListServiceDependencies(ctx context.Context, project, environment, serviceName string) ([]query.ServiceDependency, error) {
	cutoff, err := service.retention.Cutoff(ctx, project)
	if err != nil {
		return nil, err
	}
	return service.traces.ListServiceDependencies(ctx, project, environment, serviceName, cutoff)
}

func (service *Service) ListTopology(ctx context.Context, project string, filter analysis.Filter) (analysis.Topology, error) {
	cutoff, err := service.retention.Cutoff(ctx, project)
	if err != nil {
		return analysis.Topology{}, err
	}
	filter.Since = cutoff
	return service.analytics.ListTopology(ctx, project, filter)
}

func (service *Service) ListMetricPoints(ctx context.Context, project string, filter analysis.Filter) ([]analysis.MetricPoint, error) {
	cutoff, err := service.retention.Cutoff(ctx, project)
	if err != nil {
		return nil, err
	}
	filter.From = later(filter.From, cutoff)
	return service.analytics.ListMetricPoints(ctx, project, filter)
}

func (service *Service) TelemetryQuality(ctx context.Context, project string, filter analysis.Filter) (analysis.Quality, error) {
	cutoff, err := service.retention.Cutoff(ctx, project)
	if err != nil {
		return analysis.Quality{}, err
	}
	quality := analysis.Quality{From: filter.From, To: filter.To}
	filter.Since = cutoff
	if quality.Spans, quality.OrphanSpans, quality.MetricPoints, err = service.analytics.SignalQuality(ctx, project, filter); err != nil {
		return quality, err
	}
	quality.Logs, quality.UncorrelatedLogs, err = service.logs.LogQuality(ctx, project, filter.EnvironmentID, filter.Service, later(filter.From, cutoff), filter.To)
	return quality, err
}

func (service *Service) RequestPerformance(ctx context.Context, project string, filter query.PerformanceFilter) (query.RequestPerformance, error) {
	cutoff, err := service.retention.Cutoff(ctx, project)
	if err != nil {
		return query.RequestPerformance{}, err
	}
	filter.Since = cutoff
	return service.analytics.RequestPerformance(ctx, project, filter)
}

// GetIncidentAnalysis ranks RCA hypotheses from the incident's latest evidence,
// stored in PostgreSQL, and the dependencies ClickHouse observed in its window.
// A missing incident yields an empty analysis.
func (service *Service) GetIncidentAnalysis(ctx context.Context, project, environment, incidentID string) (analysis.IncidentAnalysis, error) {
	serviceName, evidence, found, err := service.incidents.IncidentEvidence(ctx, project, environment, incidentID)
	if err != nil || !found {
		return analysis.IncidentAnalysis{}, err
	}
	dependencies := []analysis.TopologyEdge{}
	if len(evidence) > 1 {
		from, to := analysis.EvidenceWindow(evidence, analysisWindow)
		topology, err := service.ListTopology(ctx, project, analysis.Filter{EnvironmentID: environment, From: from, To: to})
		if err != nil {
			return analysis.IncidentAnalysis{}, err
		}
		dependencies = topology.Edges
	}
	return analysis.BuildRCA(incidentID, serviceName, evidence, dependencies), nil
}
