package otlp

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/flight-observability/flight/internal/config"
	flightmetrics "github.com/flight-observability/flight/internal/metrics"
	"github.com/flight-observability/flight/internal/pipeline"
	"github.com/flight-observability/flight/internal/telemetry"
	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

func TestOTLPHTTPLogsPersistBeforeSuccess(t *testing.T) {
	stored := make(chan telemetry.Event, 1)
	metricRegistry := flightmetrics.New("test", "test", "test")
	ingestionPipeline := pipeline.New(4, 1<<20, 1, 1, time.Second, time.Second, time.Millisecond, func(_ context.Context, events []telemetry.Event) error {
		stored <- events[0]
		return nil
	}, metricRegistry)
	ingestionPipeline.Start()
	ingestionConfiguration := testIngestionConfig()
	ingestionConfiguration.MaxRequestBytes = 1 << 20
	receiver := NewReceiver(ingestionConfiguration, ingestionPipeline, metricRegistry)
	server := NewServer(config.Config{
		Ingestion:       ingestionConfiguration,
		HTTP:            config.HTTPConfig{MaxHeaderBytes: 1 << 20},
		ShutdownTimeout: time.Second,
	}, receiver)

	requestMessage := &collectorlogspb.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKeyValue("service.name", "worker")}},
		ScopeLogs: []*logspb.ScopeLogs{{LogRecords: []*logspb.LogRecord{{
			TimeUnixNano: uint64(time.Now().UTC().UnixNano()),
			Body:         &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "hello"}},
		}}}},
	}}}
	body, marshalError := proto.Marshal(requestMessage)
	if marshalError != nil {
		t.Fatal(marshalError)
	}
	httpRequest := httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader(body))
	httpRequest.Header.Set("Content-Type", "application/x-protobuf")
	responseRecorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(responseRecorder, httpRequest)
	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", responseRecorder.Code, responseRecorder.Body.String())
	}
	if event := <-stored; event.Type != telemetry.EventLog || event.ServiceName != "worker" {
		t.Fatalf("stored event = %+v", event)
	}

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
	defer cancelShutdown()
	if shutdownError := ingestionPipeline.Shutdown(shutdownContext); shutdownError != nil {
		t.Fatal(shutdownError)
	}
}

func TestOTLPGRPCTracesPersistBeforeSuccess(t *testing.T) {
	stored := make(chan telemetry.Event, 1)
	metricRegistry := flightmetrics.New("test", "test", "test")
	ingestionPipeline := pipeline.New(4, 1<<20, 1, 1, time.Second, time.Second, time.Millisecond, func(_ context.Context, events []telemetry.Event) error {
		stored <- events[0]
		return nil
	}, metricRegistry)
	ingestionPipeline.Start()
	ingestionConfiguration := testIngestionConfig()
	ingestionConfiguration.MaxRequestBytes = 1 << 20
	server := NewServer(config.Config{Ingestion: ingestionConfiguration}, NewReceiver(ingestionConfiguration, ingestionPipeline, metricRegistry))
	listener := bufconn.Listen(1 << 20)
	go func() { _ = server.grpcServer.Serve(listener) }()
	t.Cleanup(server.grpcServer.Stop)

	connection, connectionError := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if connectionError != nil {
		t.Fatal(connectionError)
	}
	defer connection.Close()
	startedAt := time.Now().UTC()
	request := &collectortracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKeyValue("service.name", "api")}},
		ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{
			TraceId:           bytes.Repeat([]byte{1}, 16),
			SpanId:            bytes.Repeat([]byte{2}, 8),
			Name:              "GET /",
			StartTimeUnixNano: uint64(startedAt.UnixNano()),
			EndTimeUnixNano:   uint64(startedAt.Add(time.Millisecond).UnixNano()),
		}}}},
	}}}
	if _, exportError := collectortracepb.NewTraceServiceClient(connection).Export(context.Background(), request); exportError != nil {
		t.Fatal(exportError)
	}
	if event := <-stored; event.ServiceName != "api" {
		t.Fatalf("stored event = %+v", event)
	}

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
	defer cancelShutdown()
	if shutdownError := ingestionPipeline.Shutdown(shutdownContext); shutdownError != nil {
		t.Fatal(shutdownError)
	}
}

func TestOTLPHTTPRejectsOversizedBody(t *testing.T) {
	server := NewServer(config.Config{
		Ingestion: config.IngestionConfig{MaxRequestBytes: 4},
	}, nil)
	httpRequest := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader([]byte("12345")))
	httpRequest.Header.Set("Content-Type", "application/x-protobuf")
	responseRecorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(responseRecorder, httpRequest)
	if responseRecorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d", responseRecorder.Code)
	}
}

func TestServerReadinessTracksBothListeners(t *testing.T) {
	server := NewServer(config.Config{
		Ingestion: config.IngestionConfig{
			GRPCAddress:     "127.0.0.1:0",
			HTTPAddress:     "127.0.0.1:0",
			MaxRequestBytes: 1024,
		},
		ShutdownTimeout: time.Second,
	}, nil)
	runtimeContext, cancelRuntime := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- server.Run(runtimeContext) }()
	readyDeadline := time.Now().Add(time.Second)
	for !server.Ready() && time.Now().Before(readyDeadline) {
		time.Sleep(time.Millisecond)
	}
	if !server.Ready() {
		t.Fatal("OTLP server never became ready")
	}
	cancelRuntime()
	if runError := <-result; runError != nil {
		t.Fatal(runError)
	}
	if server.Ready() {
		t.Fatal("OTLP server remained ready after shutdown")
	}
}
