package otlp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/ingest/otlp/normalize/normalizetest"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"
)

type captureSubmitter struct{ events []telemetry.Event }

func (submitter *captureSubmitter) Submit(_ context.Context, events []telemetry.Event) (int, int, error) {
	submitter.events = append(submitter.events, events...)
	return len(events), 0, nil
}

// scopedServer serves two projects from one receiver: a key belongs to the project it names, and a
// project has the environments listed here.
func scopedServer(t *testing.T) (*Server, *captureSubmitter) {
	t.Helper()
	submitter := &captureSubmitter{}
	configuration := normalizetest.Config()
	configuration.MaxRequestBytes = 1 << 20
	keys := map[string]auth.Principal{
		"key-alpha":   {ProjectID: "project-alpha", Scopes: []string{"ingest"}},
		"key-beta":    {ProjectID: "project-beta", Scopes: []string{"ingest"}},
		"key-reader":  {ProjectID: "project-alpha", Scopes: []string{"read"}},
		"key-default": {ProjectID: configuration.ProjectID, Scopes: []string{"ingest"}},
	}
	environments := map[string][]string{"project-alpha": {"production", "staging"}, "project-beta": {"production"}}
	receiver := NewReceiver(configuration, submitter, nil, metrics.New("test", "test", "test"), func(_ context.Context, key string) (auth.Principal, error) {
		if principal, ok := keys[key]; ok {
			return principal, nil
		}
		return auth.Principal{}, auth.ErrUnauthorized
	}).WithScopes(func(_ context.Context, projectID, environment string) (config.IngestionConfig, error) {
		available := environments[projectID]
		if environment == "" && len(available) > 0 {
			environment = available[0]
		}
		for _, name := range available {
			if name == environment {
				scope := configuration
				scope.ProjectID, scope.EnvironmentName, scope.EnvironmentID = projectID, name, projectID+"/"+name
				return scope, nil
			}
		}
		return config.IngestionConfig{}, ErrUnknownEnvironment
	})
	return NewServer(config.Config{Ingestion: configuration, HTTP: config.HTTPConfig{MaxHeaderBytes: 1 << 20}, App: config.AppConfig{ShutdownTimeout: time.Second}}, receiver), submitter
}

func postLog(t *testing.T, server *Server, key, environment string) *httptest.ResponseRecorder {
	t.Helper()
	body, marshalError := proto.Marshal(&collectorlogspb.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{normalizetest.StringKeyValue("service.name", "worker")}},
		ScopeLogs: []*logspb.ScopeLogs{{LogRecords: []*logspb.LogRecord{{
			TimeUnixNano: uint64(time.Now().UTC().UnixNano()),
			Body:         &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "hello"}},
		}}}},
	}}})
	if marshalError != nil {
		t.Fatal(marshalError)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/x-protobuf")
	request.Header.Set("Authorization", "Bearer "+key)
	if environment != "" {
		request.Header.Set("X-Environment", environment)
	}
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, request)
	return recorder
}

func TestKeysStoreTelemetryInTheirOwnProject(t *testing.T) {
	server, submitter := scopedServer(t)
	for _, key := range []string{"key-alpha", "key-beta"} {
		if recorder := postLog(t, server, key, ""); recorder.Code != http.StatusOK {
			t.Fatalf("%s: status %d body %s", key, recorder.Code, recorder.Body)
		}
	}
	if len(submitter.events) != 2 || submitter.events[0].ProjectID != "project-alpha" || submitter.events[1].ProjectID != "project-beta" {
		t.Fatalf("each key's telemetry must carry its own project: %+v", submitter.events)
	}
	if submitter.events[0].Environment != "production" {
		t.Fatalf("without a request, telemetry goes to the project's first environment, got %q", submitter.events[0].Environment)
	}
}

func TestSendersChooseTheEnvironment(t *testing.T) {
	server, submitter := scopedServer(t)
	if recorder := postLog(t, server, "key-alpha", "staging"); recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body)
	}
	if len(submitter.events) != 1 || submitter.events[0].Environment != "staging" {
		t.Fatalf("events = %+v", submitter.events)
	}
	recorder := postLog(t, server, "key-beta", "staging")
	if recorder.Code != http.StatusBadRequest || len(submitter.events) != 1 {
		t.Fatalf("an environment the project lacks: status %d, stored %d events; nothing may be stored", recorder.Code, len(submitter.events))
	}
}

func TestKeysWithoutTheIngestScopeAreRefused(t *testing.T) {
	server, submitter := scopedServer(t)
	if recorder := postLog(t, server, "key-reader", ""); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("a read-only key: status %d, want 401", recorder.Code)
	}
	if recorder := postLog(t, server, "nonsense", ""); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("an unknown key: status %d, want 401", recorder.Code)
	}
	if len(submitter.events) != 0 {
		t.Fatalf("refused requests stored %d events", len(submitter.events))
	}
}
