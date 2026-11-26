package otlp

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/ingest/otlp/normalize/normalizetest"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
)

type unreadBody struct{ read bool }

func (body *unreadBody) Read([]byte) (int, error) {
	body.read = true
	return 0, errors.New("unauthorized body must not be read")
}

func (*unreadBody) Close() error { return nil }

func TestOTLPHTTPAuthenticatesBeforeReadingBody(t *testing.T) {
	configuration := normalizetest.Config()
	receiver := NewReceiver(configuration, nil, nil, metrics.New("test", "test", "test"), func(context.Context, string) (auth.Principal, error) {
		return auth.Principal{}, auth.ErrUnauthorized
	})
	server := NewServer(config.Config{Ingestion: configuration}, receiver)
	for _, signal := range []string{"traces", "logs", "metrics"} {
		t.Run(signal, func(t *testing.T) {
			body := &unreadBody{}
			request := httptest.NewRequest(http.MethodPost, "/v1/"+signal, body)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			server.httpServer.Handler.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized || body.read {
				t.Fatalf("status=%d, body read=%t", response.Code, body.read)
			}
		})
	}
}

func TestOTLPAuthenticationOutageIsRetryable(t *testing.T) {
	configuration := normalizetest.Config()
	receiver := NewReceiver(configuration, nil, nil, metrics.New("test", "test", "test"), func(context.Context, string) (auth.Principal, error) {
		return auth.Principal{}, errors.New("temporary database outage")
	})
	server := NewServer(config.Config{Ingestion: configuration}, receiver)
	for _, signal := range []string{"traces", "logs", "metrics"} {
		request := httptest.NewRequest(http.MethodPost, "/v1/"+signal, strings.NewReader("{}"))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusServiceUnavailable || response.Header().Get("Retry-After") == "" {
			t.Fatalf("%s auth outage was not retryable: status=%d", signal, response.Code)
		}
	}
	_, traceError := (&traceService{receiver: receiver}).Export(context.Background(), &collectortracepb.ExportTraceServiceRequest{})
	_, logError := (&logsService{receiver: receiver}).Export(context.Background(), &collectorlogspb.ExportLogsServiceRequest{})
	_, metricError := (&metricsService{receiver: receiver}).Export(context.Background(), &collectormetricspb.ExportMetricsServiceRequest{})
	for _, err := range []error{traceError, logError, metricError} {
		if status.Code(err) != codes.Unavailable || strings.Contains(err.Error(), "database") {
			t.Fatalf("gRPC auth outage was not safely retryable: %v", err)
		}
	}
}

func TestOTLPJSONDecodesProtocolIDsAndIgnoresUnknownFields(t *testing.T) {
	const traceID = "0102030405060708090A0B0C0D0E0F10"
	const spanID = "0102030405060708"
	const timestamp = "1791507600123456789"
	cases := []struct {
		name    string
		payload string
		message proto.Message
	}{
		{name: "logs", payload: `{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"traceId":"` + traceID + `","spanId":"` + spanID + `","timeUnixNano":"` + timestamp + `","severityNumber":17,"body":{"bytesValue":"AQID"},"futureField":{"traceId":"ignored"}}]}]}],"futureEnvelope":true}`, message: &collectorlogspb.ExportLogsServiceRequest{}},
		{name: "traces", payload: `{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"` + traceID + `","spanId":"` + spanID + `","parentSpanId":"` + spanID + `","kind":2,"links":[{"traceId":"` + traceID + `","spanId":"` + spanID + `"}]}]}]}]}`, message: &collectortracepb.ExportTraceServiceRequest{}},
		{name: "metrics", payload: `{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"gauge":{"dataPoints":[{"asInt":"9007199254740993","exemplars":[{"traceId":"` + traceID + `","spanId":"` + spanID + `"}]}]}}]}]}]}`, message: &collectormetricspb.ExportMetricsServiceRequest{}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			server := &Server{maxRequestBytes: 1 << 20}
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(testCase.payload))
			request.Header.Set("Content-Type", "application/json")
			if _, err := server.decodeHTTP(httptest.NewRecorder(), request, testCase.message); err != nil {
				t.Fatal(err)
			}
			var actualTraceID, actualSpanID []byte
			switch message := testCase.message.(type) {
			case *collectorlogspb.ExportLogsServiceRequest:
				record := message.ResourceLogs[0].ScopeLogs[0].LogRecords[0]
				actualTraceID, actualSpanID = record.TraceId, record.SpanId
				if record.TimeUnixNano != 1791507600123456789 || !bytes.Equal(record.Body.GetBytesValue(), []byte{1, 2, 3}) {
					t.Fatal("timestamp precision or ordinary bytes changed")
				}
			case *collectortracepb.ExportTraceServiceRequest:
				span := message.ResourceSpans[0].ScopeSpans[0].Spans[0]
				actualTraceID, actualSpanID = span.TraceId, span.SpanId
				if hex.EncodeToString(span.ParentSpanId) != spanID || !bytes.Equal(span.Links[0].TraceId, span.TraceId) {
					t.Fatal("parent or linked IDs changed")
				}
			case *collectormetricspb.ExportMetricsServiceRequest:
				point := message.ResourceMetrics[0].ScopeMetrics[0].Metrics[0].GetGauge().DataPoints[0]
				actualTraceID, actualSpanID = point.Exemplars[0].TraceId, point.Exemplars[0].SpanId
				if point.GetAsInt() != 9007199254740993 {
					t.Fatal("integer precision changed")
				}
			}
			if hex.EncodeToString(actualTraceID) != strings.ToLower(traceID) || hex.EncodeToString(actualSpanID) != spanID {
				t.Fatalf("decoded IDs=%x/%x", actualTraceID, actualSpanID)
			}
		})
	}
}

func TestOTLPJSONRejectsInvalidKnownFields(t *testing.T) {
	for _, payload := range []string{
		`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"traceId":"not-hex"}]}]}]}`,
		`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"severityNumber":"SEVERITY_NUMBER_ERROR"}]}]}]}`,
		`{} {}`,
		`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":` + strings.Repeat(`{"arrayValue":{"values":[`, 70) + `{"stringValue":"deep"}` + strings.Repeat(`]}}`, 70) + `}]}]}]}`,
	} {
		server := &Server{maxRequestBytes: 1 << 20}
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		if _, err := server.decodeHTTP(response, request, &collectorlogspb.ExportLogsServiceRequest{}); err == nil || response.Code != http.StatusBadRequest {
			t.Fatalf("invalid payload accepted: status=%d", response.Code)
		}
	}
}

// eventCollector stores every event except the trailing `invalid` ones, which
// it reports as rejected by storage.
type eventCollector struct {
	events  []telemetry.Event
	invalid int
}

func (collector *eventCollector) Submit(_ context.Context, events []telemetry.Event) (int, int, error) {
	invalid := min(collector.invalid, len(events))
	collector.events = append(collector.events, events[:len(events)-invalid]...)
	return len(events) - invalid, invalid, nil
}

func TestOTLPHTTPJSONLogKeepsCorrelation(t *testing.T) {
	collector := &eventCollector{}
	configuration := normalizetest.Config()
	configuration.MaxRequestBytes = 1 << 20
	receiver := NewReceiver(configuration, collector, nil, metrics.New("test", "test", "test"), nil)
	server := NewServer(config.Config{Ingestion: configuration}, receiver)
	payload := `{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"worker"}}]},"scopeLogs":[{"logRecords":[{"timeUnixNano":"1791507600123456789","traceId":"0102030405060708090a0b0c0d0e0f10","spanId":"0102030405060708","severityNumber":17,"body":{"stringValue":"failed"}}]}]}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(collector.events) != 1 || collector.events[0].TraceID != "0102030405060708090a0b0c0d0e0f10" {
		t.Fatalf("status=%d, events=%+v, body=%s", response.Code, collector.events, response.Body.String())
	}
}

func TestOTLPRejectsRequestsBeyondConcurrencyLimitBeforeReadingBody(t *testing.T) {
	configuration := normalizetest.Config()
	configuration.MaxConcurrentRequests = 1
	receiver := NewReceiver(configuration, nil, nil, metrics.New("test", "test", "test"), func(context.Context, string) (auth.Principal, error) {
		return auth.Principal{ProjectID: configuration.ProjectID, Scopes: []string{"ingest"}}, nil
	})
	server := NewServer(config.Config{Ingestion: configuration}, receiver)
	release, admitted := receiver.admit()
	if !admitted {
		t.Fatal("first request was not admitted")
	}
	body := &unreadBody{}
	request := httptest.NewRequest(http.MethodPost, "/v1/logs", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("Retry-After") == "" || body.read {
		t.Fatalf("overload was not retryable without reading the body: status=%d body read=%t", response.Code, body.read)
	}
	if _, grpcError := (&logsService{receiver: receiver}).Export(context.Background(), &collectorlogspb.ExportLogsServiceRequest{}); status.Code(grpcError) != codes.Unavailable {
		t.Fatalf("gRPC overload code = %v", status.Code(grpcError))
	}

	release()
	if again, admittedAgain := receiver.admit(); !admittedAgain {
		t.Fatal("slot was not released")
	} else {
		again()
	}
}

func TestOTLPStorageRejectionIsPartialSuccessNotRetry(t *testing.T) {
	collector := &eventCollector{invalid: 1}
	configuration := normalizetest.Config()
	configuration.MaxRequestBytes = 1 << 20
	server := NewServer(config.Config{Ingestion: configuration}, NewReceiver(configuration, collector, nil, metrics.New("test", "test", "test"), nil))
	payload := `{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"worker"}}]},"scopeLogs":[{"logRecords":[{"timeUnixNano":"1791507600123456789","body":{"stringValue":"kept"}},{"timeUnixNano":"1791507600123456790","body":{"stringValue":"rejected"}}]}]}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)

	var body collectorlogspb.ExportLogsServiceResponse
	if response.Code != http.StatusOK || protojson.Unmarshal(response.Body.Bytes(), &body) != nil {
		t.Fatalf("status=%d body=%s; invalid records must not trigger a retry", response.Code, response.Body.String())
	}
	if body.GetPartialSuccess().GetRejectedLogRecords() != 1 || len(collector.events) != 1 {
		t.Fatalf("partial success = %+v, stored %d", body.GetPartialSuccess(), len(collector.events))
	}
}

// metricsStub answers SubmitMetrics with a fixed outcome.
type metricsStub struct {
	persisted, rejected int
	err                 error
}

func (stub metricsStub) SubmitMetrics(context.Context, []telemetry.MetricDataPoint) (int, int, error) {
	return stub.persisted, stub.rejected, stub.err
}

func TestOTLPMetricsAreAcknowledgedOnlyForStoredPoints(t *testing.T) {
	payload := `{"resourceMetrics":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"api"}}]},"scopeMetrics":[{"metrics":[{"name":"requests","gauge":{"dataPoints":[{"timeUnixNano":"1791507600123456789","asDouble":1}]}}]}]}]}`
	for _, test := range []struct {
		name           string
		stub           metricsStub
		status         int
		rejectedPoints int64
	}{
		{"stored", metricsStub{persisted: 1}, http.StatusOK, 0},
		{"storage outage is retryable", metricsStub{err: errors.New("database unavailable")}, http.StatusServiceUnavailable, 0},
		{"unstorable point is not retried", metricsStub{rejected: 1}, http.StatusOK, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			configuration := normalizetest.Config()
			configuration.MaxRequestBytes = 1 << 20
			server := NewServer(config.Config{Ingestion: configuration}, NewReceiver(configuration, &eventCollector{}, test.stub, metrics.New("test", "test", "test"), nil))
			request := httptest.NewRequest(http.MethodPost, "/v1/metrics", strings.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			server.httpServer.Handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.status, response.Body.String())
			}
			if test.status != http.StatusOK {
				return
			}
			var body collectormetricspb.ExportMetricsServiceResponse
			if err := protojson.Unmarshal(response.Body.Bytes(), &body); err != nil || body.GetPartialSuccess().GetRejectedDataPoints() != test.rejectedPoints {
				t.Fatalf("partial success = %+v (%v), want %d rejected", body.GetPartialSuccess(), err, test.rejectedPoints)
			}
		})
	}
}
