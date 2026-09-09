package otlp

import (
	"testing"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func TestMetricPointIDsDistinguishSamplesAndEnvironment(t *testing.T) {
	configuration := testIngestionConfig()
	normalizer := newNormalizer(configuration)
	timestamp := time.Date(2026, 8, 20, 12, 0, 0, 123456789, time.UTC)
	request := []*metricspb.ResourceMetrics{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKeyValue("service.name", "checkout")}},
		ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{{
			Name: "checkout.requests",
			Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{
				{TimeUnixNano: uint64(timestamp.UnixNano()), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: 1}},
				{TimeUnixNano: uint64(timestamp.UnixNano()), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: 2}},
			}}},
		}}}},
	}}
	points, rejected := normalizer.metrics(request)
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
	otherPoints, _ := newNormalizer(other).metrics(request)
	if len(otherPoints) != 2 || otherPoints[0].ID == points[0].ID {
		t.Fatal("same sample in another environment reused the id")
	}
}

func TestMetricAttributeMergeDoesNotMutateResourceSlice(t *testing.T) {
	normalizer := newNormalizer(testIngestionConfig())
	resourceAttributes := []*commonpb.KeyValue{stringKeyValue("service.name", "checkout")}
	resource := &resourcepb.Resource{Attributes: resourceAttributes[:1:1]}
	metric := &metricspb.Metric{
		Name: "checkout.requests",
		Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{{
			TimeUnixNano: uint64(time.Now().UnixNano()),
			Attributes:   []*commonpb.KeyValue{stringKeyValue("region", "sg")},
			Value:        &metricspb.NumberDataPoint_AsInt{AsInt: 1},
		}}}},
	}
	if points, rejected := normalizer.extractDataPoints(resource, "", nil, metric, "checkout", "", map[string]any{"service.name": "checkout"}); rejected != nil || len(points) != 1 {
		t.Fatalf("points=%d rejected=%v", len(points), rejected)
	}
	if len(resourceAttributes) != 1 || resourceAttributes[0].GetKey() != "service.name" {
		t.Fatalf("resource attributes were mutated: %#v", resourceAttributes)
	}
}
