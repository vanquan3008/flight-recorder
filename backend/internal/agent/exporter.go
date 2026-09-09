package agent

import (
	"context"
	"crypto/tls"

	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type Exporter struct {
	connection         *grpc.ClientConn
	client             collectorlogspb.LogsServiceClient
	apiKey             string
	resourceAttributes []*commonpb.KeyValue
}

func NewExporter(endpoint, apiKey, service, hostName, sourcePath string, secure bool, serverName string) (*Exporter, error) {
	transportCredentials := grpc.WithTransportCredentials(insecure.NewCredentials())
	if secure {
		transportCredentials = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName}))
	}
	connection, connectionError := grpc.NewClient(endpoint, transportCredentials)
	if connectionError != nil {
		return nil, connectionError
	}
	return &Exporter{
		connection: connection,
		client:     collectorlogspb.NewLogsServiceClient(connection),
		apiKey:     apiKey,
		resourceAttributes: []*commonpb.KeyValue{
			{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: service}}},
			{Key: "host.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: hostName}}},
			{Key: "log.file.path", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: sourcePath}}},
			{Key: "log.source.type", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "file"}}},
		},
	}, nil
}

func (exporter *Exporter) Close() error {
	return exporter.connection.Close()
}

func (exporter *Exporter) Export(ctx context.Context, batch Batch) error {
	records := make([]*logspb.LogRecord, 0, len(batch))
	for _, record := range batch {
		records = append(records, &logspb.LogRecord{TimeUnixNano: record.TimeUnixNano, Body: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: record.Body}}})
	}
	if exporter.apiKey != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-api-key", exporter.apiKey)
	}
	_, exportError := exporter.client.Export(ctx, &collectorlogspb.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{Resource: &resourcepb.Resource{Attributes: exporter.resourceAttributes}, ScopeLogs: []*logspb.ScopeLogs{{LogRecords: records}}}}})
	return exportError
}
