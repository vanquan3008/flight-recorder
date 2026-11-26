package otlp

import (
	"context"
	"errors"
	"net/netip"

	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/ingest/otlp/normalize"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
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
	normalizer      *normalize.Normalizer
	projectID       string
	scopes          ScopeResolver
	pipeline        Submitter
	metricsPipeline MetricsSubmitter
	metrics         *metrics.Registry
	authenticate    func(context.Context, string) (auth.Principal, error)
	guard           *authGuard
	trustedProxies  []netip.Prefix
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
		normalizer:      normalize.New(configuration),
		projectID:       configuration.ProjectID,
		pipeline:        ingestionPipeline,
		metricsPipeline: metricsPipeline,
		metrics:         metricRegistry,
		authenticate:    authenticate,
		guard:           newAuthGuard(configuration.AuthFailureLimit, configuration.AuthFailureWindow),
	}
}

// WithTrustedProxies names the reverse proxies whose X-Forwarded-For is
// believed, so failed-authentication budgets are kept per real client instead
// of per proxy. Without it the connection's own address is always used.
func (receiver *Receiver) WithTrustedProxies(proxies []netip.Prefix) *Receiver {
	receiver.trustedProxies = proxies
	return receiver
}

// clientAddress attributes a request to a client; see config.ClientAddress.
func (receiver *Receiver) clientAddress(remoteAddr string, forwarded []string) string {
	return config.ClientAddress(remoteAddr, forwarded, receiver.trustedProxies)
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

func (receiver *Receiver) exportTraces(ctx context.Context, request *collectortracepb.ExportTraceServiceRequest) (*collectortracepb.ExportTraceServiceResponse, error) {
	events, rejected := receiver.normalizerFor(ctx).Traces(request.GetResourceSpans())
	persisted, invalid, submitError := receiver.pipeline.Submit(ctx, events)
	if settleError := receiver.settle("traces", persisted, rejected, invalid, submitError); settleError != nil {
		return nil, settleError
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
	events, rejected := receiver.normalizerFor(ctx).Logs(request.GetResourceLogs())
	persisted, invalid, submitError := receiver.pipeline.Submit(ctx, events)
	if settleError := receiver.settle("logs", persisted, rejected, invalid, submitError); settleError != nil {
		return nil, settleError
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

// settle records committed events, records rejected by validation before queueing, and records
// storage rejected as invalid after queueing. Partial success tells an OTLP client not to retry, and
// storage and admission failures are temporary, so a submit error fails the whole request and relies
// on stable IDs for the retry.
func (receiver *Receiver) settle(signal string, persisted, rejected, invalid int, submitError error) error {
	receiver.metrics.ObserveIngested(signal, persisted)
	if rejected > 0 {
		receiver.metrics.ObserveRejected(signal, "validation", rejected)
	}
	if invalid > 0 {
		receiver.metrics.ObserveRejected(signal, "invalid", invalid)
	}
	if submitError != nil {
		return errors.New(signal + " ingestion unavailable")
	}
	return nil
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

func (service *traceService) Export(ctx context.Context, request *collectortracepb.ExportTraceServiceRequest) (*collectortracepb.ExportTraceServiceResponse, error) {
	return handleGRPC(ctx, service.receiver, "traces", func(ctx context.Context) (*collectortracepb.ExportTraceServiceResponse, error) {
		return service.receiver.exportTraces(ctx, request)
	})
}

type logsService struct {
	collectorlogspb.UnimplementedLogsServiceServer
	receiver *Receiver
}

func (service *logsService) Export(ctx context.Context, request *collectorlogspb.ExportLogsServiceRequest) (*collectorlogspb.ExportLogsServiceResponse, error) {
	return handleGRPC(ctx, service.receiver, "logs", func(ctx context.Context) (*collectorlogspb.ExportLogsServiceResponse, error) {
		return service.receiver.exportLogs(ctx, request)
	})
}

func (receiver *Receiver) exportMetrics(ctx context.Context, request *collectormetricspb.ExportMetricsServiceRequest) (*collectormetricspb.ExportMetricsServiceResponse, error) {
	if receiver.metricsPipeline == nil {
		// Metrics pipeline not configured — accept silently to avoid breaking
		// senders that include metrics alongside traces/logs.
		return &collectormetricspb.ExportMetricsServiceResponse{}, nil
	}
	points, rejected := receiver.normalizerFor(ctx).Metrics(request.GetResourceMetrics())
	persisted, invalid, submitError := receiver.metricsPipeline.SubmitMetrics(ctx, points)
	if settleError := receiver.settle("metrics", persisted, rejected, invalid, submitError); settleError != nil {
		return nil, settleError
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

func (service *metricsService) Export(ctx context.Context, request *collectormetricspb.ExportMetricsServiceRequest) (*collectormetricspb.ExportMetricsServiceResponse, error) {
	return handleGRPC(ctx, service.receiver, "metrics", func(ctx context.Context) (*collectormetricspb.ExportMetricsServiceResponse, error) {
		return service.receiver.exportMetrics(ctx, request)
	})
}
