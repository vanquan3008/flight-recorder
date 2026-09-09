package otlp

import (
	"context"
	"crypto/tls"
	"strings"

	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

func newGRPCServer(receiver *Receiver, maxRecvMsgSize int, tlsConfig *tls.Config) *grpc.Server {
	options := []grpc.ServerOption{grpc.MaxRecvMsgSize(maxRecvMsgSize)}
	if tlsConfig != nil {
		options = append(options, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}
	grpcServer := grpc.NewServer(options...)
	collectortracepb.RegisterTraceServiceServer(grpcServer, &traceService{receiver: receiver})
	collectorlogspb.RegisterLogsServiceServer(grpcServer, &logsService{receiver: receiver})
	collectormetricspb.RegisterMetricsServiceServer(grpcServer, &metricsService{receiver: receiver})
	return grpcServer
}

func grpcAPIKey(ctx context.Context) string {
	metadataValues, exists := metadata.FromIncomingContext(ctx)
	if !exists {
		return ""
	}
	if values := metadataValues.Get("x-api-key"); len(values) > 0 {
		return values[0]
	}
	if values := metadataValues.Get("authorization"); len(values) > 0 && strings.HasPrefix(values[0], "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(values[0], "Bearer "))
	}
	return ""
}
