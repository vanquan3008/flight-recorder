package otlp

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/ingest/otlp/normalize/normalizetest"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/ingest/pipeline"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestOTLPHTTPLogsPersistBeforeSuccess(t *testing.T) {
	stored := make(chan telemetry.Event, 1)
	metricRegistry := metrics.New("test", "test", "test")
	ingestionPipeline := pipeline.New(pipeline.Config{QueueSize: 4, QueueMaxBytes: 1 << 20, BatchSize: 1, Workers: 1, EnqueueTimeout: time.Second, BatchTimeout: time.Second, FlushInterval: time.Millisecond}, func(_ context.Context, events []telemetry.Event) error {
		stored <- events[0]
		return nil
	}, metricRegistry)
	ingestionPipeline.Start()
	ingestionConfiguration := normalizetest.Config()
	ingestionConfiguration.MaxRequestBytes = 1 << 20
	receiver := NewReceiver(ingestionConfiguration, ingestionPipeline, nil, metricRegistry, nil)
	server := NewServer(config.Config{
		Ingestion: ingestionConfiguration,
		HTTP:      config.HTTPConfig{MaxHeaderBytes: 1 << 20},
		App:       config.AppConfig{ShutdownTimeout: time.Second},
	}, receiver)

	requestMessage := &collectorlogspb.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{normalizetest.StringKeyValue("service.name", "worker")}},
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
	metricRegistry := metrics.New("test", "test", "test")
	ingestionPipeline := pipeline.New(pipeline.Config{QueueSize: 4, QueueMaxBytes: 1 << 20, BatchSize: 1, Workers: 1, EnqueueTimeout: time.Second, BatchTimeout: time.Second, FlushInterval: time.Millisecond}, func(_ context.Context, events []telemetry.Event) error {
		stored <- events[0]
		return nil
	}, metricRegistry)
	ingestionPipeline.Start()
	ingestionConfiguration := normalizetest.Config()
	ingestionConfiguration.MaxRequestBytes = 1 << 20
	server := NewServer(config.Config{Ingestion: ingestionConfiguration}, NewReceiver(ingestionConfiguration, ingestionPipeline, nil, metricRegistry, nil))
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
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{normalizetest.StringKeyValue("service.name", "api")}},
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
	_, _ = server.decodeHTTP(responseRecorder, httpRequest, &collectortracepb.ExportTraceServiceRequest{})
	if responseRecorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d", responseRecorder.Code)
	}
	if responseRecorder.Header().Get("Content-Type") != "application/x-protobuf" {
		t.Fatalf("content type = %q", responseRecorder.Header().Get("Content-Type"))
	}
	responseStatus := &statuspb.Status{}
	if unmarshalError := proto.Unmarshal(responseRecorder.Body.Bytes(), responseStatus); unmarshalError != nil {
		t.Fatal(unmarshalError)
	}
	if responseStatus.GetMessage() != "OTLP request body too large" {
		t.Fatalf("status message = %q", responseStatus.GetMessage())
	}
}

func TestOTLPHTTPJSONErrorsUseJSONStatus(t *testing.T) {
	server := NewServer(config.Config{Ingestion: config.IngestionConfig{MaxRequestBytes: 1024}}, nil)
	httpRequest := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewBufferString("not JSON"))
	httpRequest.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()
	_, _ = server.decodeHTTP(responseRecorder, httpRequest, &collectortracepb.ExportTraceServiceRequest{})
	if responseRecorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", responseRecorder.Code)
	}
	if responseRecorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content type = %q", responseRecorder.Header().Get("Content-Type"))
	}
	responseStatus := &statuspb.Status{}
	if unmarshalError := protojson.Unmarshal(responseRecorder.Body.Bytes(), responseStatus); unmarshalError != nil {
		t.Fatal(unmarshalError)
	}
	if responseStatus.GetMessage() != "invalid OTLP JSON" {
		t.Fatalf("status message = %q", responseStatus.GetMessage())
	}
}

func TestOTLPHTTPStorageFailureIsRetryable(t *testing.T) {
	metricRegistry := metrics.New("test", "test", "test")
	ingestionPipeline := pipeline.New(pipeline.Config{QueueSize: 4, QueueMaxBytes: 1 << 20, BatchSize: 1, Workers: 1, EnqueueTimeout: time.Second, BatchTimeout: 10 * time.Millisecond, FlushInterval: time.Millisecond}, func(context.Context, []telemetry.Event) error {
		return errors.New("database unavailable")
	}, metricRegistry)
	ingestionPipeline.Start()
	t.Cleanup(func() {
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
		defer cancelShutdown()
		if shutdownError := ingestionPipeline.Shutdown(shutdownContext); shutdownError != nil {
			t.Error(shutdownError)
		}
	})

	ingestionConfiguration := normalizetest.Config()
	ingestionConfiguration.MaxRequestBytes = 1 << 20
	server := NewServer(config.Config{Ingestion: ingestionConfiguration}, NewReceiver(ingestionConfiguration, ingestionPipeline, nil, metricRegistry, nil))
	requestMessage := &collectortracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{normalizetest.StringKeyValue("service.name", "api")}},
		ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{
			TraceId:           bytes.Repeat([]byte{1}, 16),
			SpanId:            bytes.Repeat([]byte{2}, 8),
			Name:              "GET /",
			StartTimeUnixNano: uint64(time.Now().UTC().UnixNano()),
			EndTimeUnixNano:   uint64(time.Now().UTC().Add(time.Millisecond).UnixNano()),
		}}}},
	}}}
	body, marshalError := proto.Marshal(requestMessage)
	if marshalError != nil {
		t.Fatal(marshalError)
	}
	httpRequest := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
	httpRequest.Header.Set("Content-Type", "application/x-protobuf")
	responseRecorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(responseRecorder, httpRequest)
	if responseRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %q", responseRecorder.Code, responseRecorder.Body.String())
	}
	if responseRecorder.Header().Get("Retry-After") != "1" {
		t.Fatalf("retry-after = %q", responseRecorder.Header().Get("Retry-After"))
	}
}

func TestOTLPGRPCRejectsMessageAboveHTTPBodyLimit(t *testing.T) {
	server := NewServer(config.Config{Ingestion: config.IngestionConfig{MaxRequestBytes: 32}}, nil)
	listener := bufconn.Listen(1 << 20)
	go func() { _ = server.grpcServer.Serve(listener) }()
	t.Cleanup(server.grpcServer.Stop)
	connection, connectionError := grpc.NewClient("passthrough:///bufnet-limit",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if connectionError != nil {
		t.Fatal(connectionError)
	}
	defer connection.Close()
	request := &collectortracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{Name: "this payload is larger than thirty two bytes"}}}}}}}
	_, exportError := collectortracepb.NewTraceServiceClient(connection).Export(context.Background(), request)
	if status.Code(exportError) != codes.ResourceExhausted {
		t.Fatalf("gRPC oversized status = %v, error=%v", status.Code(exportError), exportError)
	}
}

func TestServerReadinessTracksBothListeners(t *testing.T) {
	server := NewServer(config.Config{
		Ingestion: config.IngestionConfig{
			GRPCAddress:     "127.0.0.1:0",
			HTTPAddress:     "127.0.0.1:0",
			MaxRequestBytes: 1024,
		},
		App: config.AppConfig{ShutdownTimeout: time.Second},
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
