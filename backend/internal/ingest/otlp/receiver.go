package otlp

import (
	"context"
	"errors"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Submitter stores trace and log events. It returns the committed count, the
// count storage rejected as invalid, and an error for retryable failures.
type Submitter interface {
	Submit(context.Context, []telemetry.Event) (persisted, rejected int, err error)
}

// MetricsSubmitter stores metric data points. It returns the committed count,
// the count storage rejected as invalid, and an error for retryable failures.
type MetricsSubmitter interface {
	SubmitMetrics(context.Context, []telemetry.MetricDataPoint) (persisted, rejected int, err error)
}

type Receiver struct {
	normalizer      *normalizer
	pipeline        Submitter
	metricsPipeline MetricsSubmitter
	metrics         *metrics.Registry
	authenticate    func(context.Context, string) (auth.Principal, error)
	// slots bounds requests that are decoding, normalizing, or waiting on the
	// pipeline. Without it every concurrent request holds its decoded payload
	// plus per-record metadata, so memory grows with client concurrency instead
	// of staying under the configured budget. nil means unlimited.
	slots chan struct{}
}

func NewReceiver(
	configuration config.IngestionConfig,
	ingestionPipeline Submitter,
	metricsPipeline MetricsSubmitter,
	metricRegistry *metrics.Registry,
	authenticate func(context.Context, string) (auth.Principal, error),
) *Receiver {
	var slots chan struct{}
	if configuration.MaxConcurrentRequests > 0 {
		slots = make(chan struct{}, configuration.MaxConcurrentRequests)
	}
	return &Receiver{
		slots:           slots,
		normalizer:      newNormalizer(configuration),
		pipeline:        ingestionPipeline,
		metricsPipeline: metricsPipeline,
		metrics:         metricRegistry,
		authenticate:    authenticate,
	}
}

// admit reserves a request slot without waiting. A rejected request is
// retryable: nothing was decoded or acknowledged on its behalf.
func (receiver *Receiver) admit() (release func(), admitted bool) {
	if receiver.slots == nil {
		return func() {}, true
	}
	select {
	case receiver.slots <- struct{}{}:
		return func() { <-receiver.slots }, true
	default:
		return nil, false
	}
}

func (receiver *Receiver) authorize(ctx context.Context, rawKey string) error {
	if receiver.authenticate == nil {
		return nil
	}
	principal, authError := receiver.authenticate(ctx, rawKey)
	if authError != nil {
		return authError
	}
	if principal.ProjectID != receiver.normalizer.configuration.ProjectID || !principal.HasScope("ingest") {
		return auth.ErrUnauthorized
	}
	return nil
}

func (receiver *Receiver) exportTraces(ctx context.Context, request *collectortracepb.ExportTraceServiceRequest) (*collectortracepb.ExportTraceServiceResponse, error) {
	events, rejected := receiver.normalizer.traces(request.GetResourceSpans())
	persisted, invalid, submitError := receiver.pipeline.Submit(ctx, events)
	receiver.observeOutcome("traces", persisted, rejected, invalid)
	if submitError != nil {
		// Partial success tells an OTLP client not to retry. Storage and admission
		// failures are temporary, so fail the whole request and rely on stable IDs.
		return nil, errors.New("trace ingestion unavailable")
	}
	response := &collectortracepb.ExportTraceServiceResponse{}
	if rejected+invalid > 0 {
		response.PartialSuccess = &collectortracepb.ExportTracePartialSuccess{
			RejectedSpans: int64(rejected + invalid),
			ErrorMessage:  rejectionMessage("spans", invalid),
		}
	}
	return response, nil
}

func (receiver *Receiver) exportLogs(ctx context.Context, request *collectorlogspb.ExportLogsServiceRequest) (*collectorlogspb.ExportLogsServiceResponse, error) {
	events, rejected := receiver.normalizer.logs(request.GetResourceLogs())
	persisted, invalid, submitError := receiver.pipeline.Submit(ctx, events)
	receiver.observeOutcome("logs", persisted, rejected, invalid)
	if submitError != nil {
		// See trace export: temporary failures must remain retryable.
		return nil, errors.New("log ingestion unavailable")
	}
	response := &collectorlogspb.ExportLogsServiceResponse{}
	if rejected+invalid > 0 {
		response.PartialSuccess = &collectorlogspb.ExportLogsPartialSuccess{
			RejectedLogRecords: int64(rejected + invalid),
			ErrorMessage:       rejectionMessage("log records", invalid),
		}
	}
	return response, nil
}

// observeOutcome records committed events, records rejected by validation
// before queueing, and records storage rejected as invalid after queueing.
func (receiver *Receiver) observeOutcome(signal string, persisted, rejected, invalid int) {
	receiver.metrics.ObserveIngested(signal, persisted)
	if rejected > 0 {
		receiver.metrics.ObserveRejected(signal, "validation", rejected)
	}
	if invalid > 0 {
		receiver.metrics.ObserveRejected(signal, "invalid", invalid)
	}
}

func rejectionMessage(records string, invalid int) string {
	if invalid > 0 {
		return "one or more " + records + " were rejected as invalid and must not be retried"
	}
	return "one or more " + records + " were rejected by validated ingestion limits"
}

type traceService struct {
	collectortracepb.UnimplementedTraceServiceServer
	receiver *Receiver
}

func (service *traceService) Export(ctx context.Context, request *collectortracepb.ExportTraceServiceRequest) (response *collectortracepb.ExportTraceServiceResponse, returnError error) {
	startedAt := time.Now()
	defer func() {
		outcome := "ok"
		if returnError != nil {
			outcome = status.Code(returnError).String()
		} else if response.GetPartialSuccess() != nil {
			outcome = "partial"
		}
		service.receiver.metrics.ObserveOTLPRequest("grpc", "traces", outcome, time.Since(startedAt))
	}()
	if authError := service.receiver.authorizeGRPC(ctx); authError != nil {
		return nil, authError
	}
	release, admitError := service.receiver.admitGRPC()
	if admitError != nil {
		return nil, admitError
	}
	defer release()
	response, exportError := service.receiver.exportTraces(ctx, request)
	if exportError != nil {
		return nil, status.Error(codes.Unavailable, exportError.Error())
	}
	return response, nil
}

type logsService struct {
	collectorlogspb.UnimplementedLogsServiceServer
	receiver *Receiver
}

func (service *logsService) Export(ctx context.Context, request *collectorlogspb.ExportLogsServiceRequest) (response *collectorlogspb.ExportLogsServiceResponse, returnError error) {
	startedAt := time.Now()
	defer func() {
		outcome := "ok"
		if returnError != nil {
			outcome = status.Code(returnError).String()
		} else if response.GetPartialSuccess() != nil {
			outcome = "partial"
		}
		service.receiver.metrics.ObserveOTLPRequest("grpc", "logs", outcome, time.Since(startedAt))
	}()
	if authError := service.receiver.authorizeGRPC(ctx); authError != nil {
		return nil, authError
	}
	release, admitError := service.receiver.admitGRPC()
	if admitError != nil {
		return nil, admitError
	}
	defer release()
	response, exportError := service.receiver.exportLogs(ctx, request)
	if exportError != nil {
		return nil, status.Error(codes.Unavailable, exportError.Error())
	}
	return response, nil
}

func (receiver *Receiver) exportMetrics(ctx context.Context, request *collectormetricspb.ExportMetricsServiceRequest) (*collectormetricspb.ExportMetricsServiceResponse, error) {
	if receiver.metricsPipeline == nil {
		// Metrics pipeline not configured — accept silently to avoid breaking
		// senders that include metrics alongside traces/logs.
		return &collectormetricspb.ExportMetricsServiceResponse{}, nil
	}
	points, rejected := receiver.normalizer.metrics(request.GetResourceMetrics())
	persisted, invalid, submitError := receiver.metricsPipeline.SubmitMetrics(ctx, points)
	receiver.observeOutcome("metrics", persisted, rejected, invalid)
	if submitError != nil {
		return nil, errors.New("metrics ingestion unavailable")
	}
	response := &collectormetricspb.ExportMetricsServiceResponse{}
	if rejected+invalid > 0 {
		response.PartialSuccess = &collectormetricspb.ExportMetricsPartialSuccess{
			RejectedDataPoints: int64(rejected + invalid),
			ErrorMessage:       rejectionMessage("data points", invalid),
		}
	}
	return response, nil
}

type metricsService struct {
	collectormetricspb.UnimplementedMetricsServiceServer
	receiver *Receiver
}

func (service *metricsService) Export(ctx context.Context, request *collectormetricspb.ExportMetricsServiceRequest) (response *collectormetricspb.ExportMetricsServiceResponse, returnError error) {
	startedAt := time.Now()
	defer func() {
		outcome := "ok"
		if returnError != nil {
			outcome = status.Code(returnError).String()
		} else if response.GetPartialSuccess() != nil {
			outcome = "partial"
		}
		service.receiver.metrics.ObserveOTLPRequest("grpc", "metrics", outcome, time.Since(startedAt))
	}()
	if authError := service.receiver.authorizeGRPC(ctx); authError != nil {
		return nil, authError
	}
	release, admitError := service.receiver.admitGRPC()
	if admitError != nil {
		return nil, admitError
	}
	defer release()
	response, exportError := service.receiver.exportMetrics(ctx, request)
	if exportError != nil {
		return nil, status.Error(codes.Unavailable, exportError.Error())
	}
	return response, nil
}
