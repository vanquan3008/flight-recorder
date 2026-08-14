package otlp

import (
	"context"
	"errors"

	"github.com/flight-observability/flight/internal/config"
	flightmetrics "github.com/flight-observability/flight/internal/metrics"
	"github.com/flight-observability/flight/internal/pipeline"
	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Receiver struct {
	normalizer *normalizer
	pipeline   *pipeline.Pipeline
	metrics    *flightmetrics.Registry
}

func NewReceiver(configuration config.IngestionConfig, ingestionPipeline *pipeline.Pipeline, metricRegistry *flightmetrics.Registry) *Receiver {
	return &Receiver{normalizer: newNormalizer(configuration), pipeline: ingestionPipeline, metrics: metricRegistry}
}

func (receiver *Receiver) exportTraces(ctx context.Context, request *collectortracepb.ExportTraceServiceRequest) (*collectortracepb.ExportTraceServiceResponse, error) {
	events, rejected := receiver.normalizer.traces(request.GetResourceSpans())
	persisted, submitError := receiver.pipeline.Submit(ctx, events)
	receiver.metrics.ObserveIngested("traces", persisted)
	if rejected > 0 {
		receiver.metrics.ObserveRejected("traces", "validation", rejected)
	}
	if submitError != nil {
		admissionRejected := len(events) - persisted
		rejected += admissionRejected
		receiver.metrics.ObserveRejected("traces", "admission_or_storage", admissionRejected)
		if persisted == 0 && len(events) > 0 {
			return nil, errors.New("trace ingestion unavailable")
		}
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
		admissionRejected := len(events) - persisted
		rejected += admissionRejected
		receiver.metrics.ObserveRejected("logs", "admission_or_storage", admissionRejected)
		if persisted == 0 && len(events) > 0 {
			return nil, errors.New("log ingestion unavailable")
		}
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

func (service *traceService) Export(ctx context.Context, request *collectortracepb.ExportTraceServiceRequest) (*collectortracepb.ExportTraceServiceResponse, error) {
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

func (service *logsService) Export(ctx context.Context, request *collectorlogspb.ExportLogsServiceRequest) (*collectorlogspb.ExportLogsServiceResponse, error) {
	response, exportError := service.receiver.exportLogs(ctx, request)
	if exportError != nil {
		return nil, status.Error(codes.Unavailable, exportError.Error())
	}
	return response, nil
}
