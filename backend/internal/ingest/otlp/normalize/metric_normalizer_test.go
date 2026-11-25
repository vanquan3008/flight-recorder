package normalize

import (
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/ingest/otlp/normalize/normalizetest"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func TestMetricPointIDsDistinguishSamplesAndEnvironment(t *testing.T) {
	configuration := normalizetest.Config()
	normalizer := New(configuration)
	timestamp := time.Date(2026, 8, 20, 12, 0, 0, 123456789, time.UTC)
	request := []*metricspb.ResourceMetrics{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{normalizetest.StringKeyValue("service.name", "checkout")}},
		ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{{
			Name: "checkout.requests",
			Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{
				{TimeUnixNano: uint64(timestamp.UnixNano()), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: 1}},
				{TimeUnixNano: uint64(timestamp.UnixNano()), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: 2}},
			}}},
		}}}},
	}}
	points, rejected := normalizer.Metrics(request)
	if rejected != 0 || len(points) != 2 {
		t.Fatalf("normalized points=%d rejected=%d", len(points), rejected)
	}
	if points[0].ID == points[1].ID {
		t.Fatal("distinct samples at one timestamp received the same id")
	}
	if points[0].EnvironmentID != configuration.EnvironmentID {
		t.Fatalf("point environment=%q, want %q", points[0].EnvironmentID, configuration.EnvironmentID)
	}
	other := configuration
	other.EnvironmentID = "01900000-0000-7000-8000-000000000099"
	otherPoints, _ := New(other).Metrics(request)
	if len(otherPoints) != 2 || otherPoints[0].ID == points[0].ID {
		t.Fatal("same sample in another environment reused the id")
	}
}

func TestMetricAttributeMergeDoesNotMutateResourceSlice(t *testing.T) {
	normalizer := New(normalizetest.Config())
	resourceAttributes := []*commonpb.KeyValue{normalizetest.StringKeyValue("service.name", "checkout")}
	resource := &resourcepb.Resource{Attributes: resourceAttributes[:1:1]}
	metric := &metricspb.Metric{
		Name: "checkout.requests",
		Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{{
			TimeUnixNano: uint64(time.Now().UnixNano()),
			Attributes:   []*commonpb.KeyValue{normalizetest.StringKeyValue("region", "sg")},
			Value:        &metricspb.NumberDataPoint_AsInt{AsInt: 1},
		}}}},
	}
	if points, rejected := normalizer.extractDataPoints(resource, "", nil, metric, "checkout", "", map[string]any{"service.name": "checkout"}, 100); rejected != 0 || len(points) != 1 {
		t.Fatalf("points=%d rejected=%d", len(points), rejected)
	}
	if len(resourceAttributes) != 1 || resourceAttributes[0].GetKey() != "service.name" {
		t.Fatalf("resource attributes were mutated: %#v", resourceAttributes)
	}
}

func TestMetricExemplarKeepsTraceCorrelation(t *testing.T) {
	timestamp := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	normalizer := New(normalizetest.Config())
	metric := &metricspb.Metric{Name: "queue.backlog", Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{{
		TimeUnixNano: uint64(timestamp.UnixNano()), Value: &metricspb.NumberDataPoint_AsInt{AsInt: 42},
		Exemplars: []*metricspb.Exemplar{{TimeUnixNano: uint64(timestamp.UnixNano()), TraceId: []byte("0123456789abcdef"), SpanId: []byte("01234567"), Value: &metricspb.Exemplar_AsInt{AsInt: 42}}},
	}}}}}
	points, rejected := normalizer.extractDataPoints(&resourcepb.Resource{}, "", nil, metric, "worker", "", map[string]any{}, 100)
	if rejected != 0 || len(points) != 1 || len(points[0].Exemplars) != 1 {
		t.Fatalf("points=%d exemplars=%d rejected=%v", len(points), len(points[0].Exemplars), rejected)
	}
	if points[0].Exemplars[0].TraceID != "30313233343536373839616263646566" {
		t.Fatalf("trace id = %q", points[0].Exemplars[0].TraceID)
	}
}
