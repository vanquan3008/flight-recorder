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
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Submitter interface {
	Submit(context.Context, []telemetry.Event) (int, error)
}

type Receiver struct {
	normalizer   *normalizer
	pipeline     Submitter
	metrics      *metrics.Registry
	authenticate func(context.Context, string) (auth.Principal, error)
}

func NewReceiver(configuration config.IngestionConfig, ingestionPipeline Submitter, metricRegistry *metrics.Registry, authenticate func(context.Context, string) (auth.Principal, error)) *Receiver {
	return &Receiver{normalizer: newNormalizer(configuration), pipeline: ingestionPipeline, metrics: metricRegistry, authenticate: authenticate}
}

func (receiver *Receiver) authorize(ctx context.Context, rawKey string) error {
	if receiver.authenticate == nil {
		return nil
	}
	principal, authError := receiver.authenticate(ctx, rawKey)
	if authError != nil || principal.ProjectID != receiver.normalizer.configuration.ProjectID || !principal.HasScope("ingest") {
		return errors.New("OTLP authentication failed")
	}
	return nil
}

func (receiver *Receiver) exportTraces(ctx context.Context, request *collectortracepb.ExportTraceServiceRequest) (*collectortracepb.ExportTraceServiceResponse, error) {
	events, rejected := receiver.normalizer.traces(request.GetResourceSpans())
	persisted, submitError := receiver.pipeline.Submit(ctx, events)
	receiver.metrics.ObserveIngested("traces", persisted)
	if rejected > 0 {
		receiver.metrics.ObserveRejected("traces", "validation", rejected)
	}
	if submitError != nil {
		// Partial success tells an OTLP client not to retry. Storage and admission
		// failures are temporary, so fail the whole request and rely on stable IDs.
		return nil, errors.New("trace ingestion unavailable")
	}
	response := &collectortracepb.ExportTraceServiceResponse{}
	if rejected > 0 {
		response.PartialSuccess = &collectortracepb.ExportTracePartialSuccess{
			RejectedSpans: int64(rejected),
			ErrorMessage:  "one or more spans were rejected by validated ingestion limits",
		}
	}
	return response, nil
}

func (receiver *Receiver) exportLogs(ctx context.Context, request *collectorlogspb.ExportLogsServiceRequest) (*collectorlogspb.ExportLogsServiceResponse, error) {
	events, rejected := receiver.normalizer.logs(request.GetResourceLogs())
	persisted, submitError := receiver.pipeline.Submit(ctx, events)
	receiver.metrics.ObserveIngested("logs", persisted)
	if rejected > 0 {
		receiver.metrics.ObserveRejected("logs", "validation", rejected)
	}
	if submitError != nil {
		// See trace export: temporary failures must remain retryable.
		return nil, errors.New("log ingestion unavailable")
	}
	response := &collectorlogspb.ExportLogsServiceResponse{}
	if rejected > 0 {
		response.PartialSuccess = &collectorlogspb.ExportLogsPartialSuccess{
			RejectedLogRecords: int64(rejected),
			ErrorMessage:       "one or more log records were rejected by validated ingestion limits",
		}
	}
	return response, nil
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
	if authError := service.receiver.authorize(ctx, grpcAPIKey(ctx)); authError != nil {
		return nil, status.Error(codes.Unauthenticated, "OTLP authentication failed")
	}
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
	if authError := service.receiver.authorize(ctx, grpcAPIKey(ctx)); authError != nil {
		return nil, status.Error(codes.Unauthenticated, "OTLP authentication failed")
	}
	response, exportError := service.receiver.exportLogs(ctx, request)
	if exportError != nil {
		return nil, status.Error(codes.Unavailable, exportError.Error())
	}
	return response, nil
}
