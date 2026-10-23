package otlp

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/auth"
	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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

func (receiver *Receiver) authorizeGRPC(ctx context.Context) error {
	if err := receiver.authorize(ctx, grpcAPIKey(ctx)); err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			return status.Error(codes.Unauthenticated, "OTLP authentication failed")
		}
		return status.Error(codes.Unavailable, "OTLP authentication temporarily unavailable")
	}
	return nil
}

// admitGRPC runs after authentication so unauthenticated callers cannot occupy
// slots. gRPC decodes the message before the handler runs, so this bounds
// normalization and pipeline work, not the framework's own receive buffers.
func (receiver *Receiver) admitGRPC() (func(), error) {
	release, admitted := receiver.admit()
	if !admitted {
		return nil, status.Error(codes.Unavailable, "OTLP receiver is at its concurrent request limit")
	}
	return release, nil
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
