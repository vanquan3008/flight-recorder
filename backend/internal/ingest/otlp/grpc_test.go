package otlp

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/ingest/otlp/normalize/normalizetest"
	"github.com/vanquan3008/tracepilot/internal/ingest/pipeline"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
)

func TestOTLPGRPCQueuesStreamsBeyondTheProcessingLimit(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	metricRegistry := metrics.New("test", "test", "test")
	ingestionPipeline := pipeline.New(pipeline.Config{QueueSize: 4, QueueMaxBytes: 1 << 20, BatchSize: 1, Workers: 1, EnqueueTimeout: 5 * time.Second, BatchTimeout: 5 * time.Second, FlushInterval: time.Millisecond}, func(_ context.Context, _ []telemetry.Event) error {
		entered <- struct{}{}
		<-release
		return nil
	}, metricRegistry)
	ingestionPipeline.Start()
	ingestionConfiguration := normalizetest.Config()
	ingestionConfiguration.MaxRequestBytes = 1 << 20
	ingestionConfiguration.MaxConcurrentRequests = 1
	server := NewServer(config.Config{Ingestion: ingestionConfiguration}, NewReceiver(ingestionConfiguration, ingestionPipeline, nil, metricRegistry, nil))
	listener := bufconn.Listen(1 << 20)
	go func() { _ = server.grpcServer.Serve(listener) }()
	t.Cleanup(server.grpcServer.Stop)

	connection, connectionError := grpc.NewClient("passthrough:///bufnet-streams",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if connectionError != nil {
		t.Fatal(connectionError)
	}
	defer connection.Close()
	client := collectortracepb.NewTraceServiceClient(connection)
	export := func(spanID byte) <-chan error {
		result := make(chan error, 1)
		startedAt := time.Now().UTC()
		request := &collectortracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{
			Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{normalizetest.StringKeyValue("service.name", "api")}},
			ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{
				TraceId:           bytes.Repeat([]byte{1}, 16),
				SpanId:            bytes.Repeat([]byte{spanID}, 8),
				Name:              "GET /",
				StartTimeUnixNano: uint64(startedAt.UnixNano()),
				EndTimeUnixNano:   uint64(startedAt.Add(time.Millisecond).UnixNano()),
			}}}},
		}}}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := client.Export(ctx, request)
			result <- err
		}()
		return result
	}

	first := export(2)
	select {
	case <-entered:
	case err := <-first:
		t.Fatalf("first export returned before storage: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("first export never reached storage")
	}
	second := export(3)
	select {
	case err := <-second:
		t.Fatalf("second export must wait for a stream instead of failing on a busy slot: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	for name, result := range map[string]<-chan error{"first": first, "second": second} {
		if err := <-result; err != nil {
			t.Fatalf("%s export failed: %v", name, err)
		}
	}

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
	defer cancelShutdown()
	if shutdownError := ingestionPipeline.Shutdown(shutdownContext); shutdownError != nil {
		t.Fatal(shutdownError)
	}
}
