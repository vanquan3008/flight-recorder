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
	"google.golang.org/grpc/peer"
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

// authorizeGRPC authenticates the caller and returns a context carrying the scope its telemetry
// belongs to. The environment comes from the x-environment metadata entry.
func (receiver *Receiver) authorizeGRPC(ctx context.Context) (context.Context, error) {
	scoped, err := receiver.authorizeScope(ctx, grpcToken(ctx), grpcEnvironment(ctx), receiver.grpcClient(ctx))
	switch {
	case err == nil:
		return scoped, nil
	case errors.Is(err, ErrThrottled):
		return ctx, status.Error(codes.ResourceExhausted, "too many failed authentication attempts")
	case errors.Is(err, auth.ErrUnauthorized):
		return ctx, status.Error(codes.Unauthenticated, "OTLP authentication failed")
	case errors.Is(err, ErrUnknownEnvironment):
		return ctx, status.Error(codes.InvalidArgument, "unknown environment for this project")
	default:
		return ctx, status.Error(codes.Unavailable, "OTLP authentication temporarily unavailable")
	}
}

func grpcEnvironment(ctx context.Context) string {
	if incoming, found := metadata.FromIncomingContext(ctx); found {
		if values := incoming.Get("x-environment"); len(values) > 0 {
			return strings.TrimSpace(values[0])
		}
	}
	return ""
}

func (receiver *Receiver) grpcClient(ctx context.Context) string {
	remote := ""
	if caller, found := peer.FromContext(ctx); found && caller.Addr != nil {
		remote = caller.Addr.String()
	}
	var forwarded []string
	if incoming, found := metadata.FromIncomingContext(ctx); found {
		forwarded = incoming.Get("x-forwarded-for")
	}
	return receiver.clientAddress(remote, forwarded)
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

func grpcToken(ctx context.Context) string {
	metadataValues, exists := metadata.FromIncomingContext(ctx)
	if !exists {
		return ""
	}
	if values := metadataValues.Get("authorization"); len(values) > 0 && strings.HasPrefix(values[0], "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(values[0], "Bearer "))
	}
	return ""
}
