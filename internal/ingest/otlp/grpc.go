package otlp

import (
	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
)

func newGRPCServer(receiver *Receiver, maxRecvMsgSize int) *grpc.Server {
	grpcServer := grpc.NewServer(grpc.MaxRecvMsgSize(maxRecvMsgSize))
	collectortracepb.RegisterTraceServiceServer(grpcServer, &traceService{receiver: receiver})
	collectorlogspb.RegisterLogsServiceServer(grpcServer, &logsService{receiver: receiver})
	return grpcServer
}
