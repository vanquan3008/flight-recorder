package otlp

import (
	"errors"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func mergeAttributes(resource, datapoint []*commonpb.KeyValue) []*commonpb.KeyValue {
	merged := make([]*commonpb.KeyValue, 0, len(resource)+len(datapoint))
	merged = append(merged, resource...)
	return append(merged, datapoint...)
}

// metrics counts every input point once, preserving valid siblings on rejection.
func (n *normalizer) metrics(resourceMetrics []*metricspb.ResourceMetrics) ([]telemetry.MetricDataPoint, int) {
	points := make([]telemetry.MetricDataPoint, 0)
	rejected, remaining := 0, n.configuration.MaxRecordsPerRequest
	for _, rm := range resourceMetrics {
		resAttrs, resourceError := n.attributeValues(rm.GetResource().GetAttributes())
		service := stringAttribute(resAttrs, "service.name")
		version := stringAttribute(resAttrs, "service.version")
		for _, sm := range rm.GetScopeMetrics() {
			for _, metric := range sm.GetMetrics() {
				count := metricPointCount(metric)
				if resourceError != nil {
					rejected += count
				} else {
					accepted, invalid := n.extractDataPoints(rm.GetResource(), rm.GetSchemaUrl(), sm.GetScope(), metric, service, version, resAttrs, max(remaining, 0))
					points = append(points, accepted...)
					rejected += invalid
				}
				remaining = max(remaining-count, 0)
			}
		}
	}
	return points, rejected
}

func (n *normalizer) extractDataPoints(resource *resourcepb.Resource, schema string, scope *commonpb.InstrumentationScope, metric *metricspb.Metric, service, version string, resourceAttrs map[string]any, limit int) ([]telemetry.MetricDataPoint, int) {
	if metric.GetName() == "" || service == "" || n.validateText(metric.GetName(), metric.GetUnit(), metric.GetDescription(), schema, scope.GetName(), scope.GetVersion()) != nil {
		return nil, metricPointCount(metric)
	}
	scopeAttrs, err := n.attributeValues(scope.GetAttributes())
	if err != nil {
		return nil, metricPointCount(metric)
	}
	base := telemetry.MetricDataPoint{
		ProjectID: n.configuration.ProjectID, EnvironmentID: n.configuration.EnvironmentID,
		MetricName: metric.GetName(), MetricUnit: metric.GetUnit(),
		ServiceName: service, ServiceVersion: version, ResourceAttributes: resourceAttrs,
		ScopeName: scope.GetName(), ScopeVersion: scope.GetVersion(), ScopeAttributes: scopeAttrs,
	}
	switch data := metric.GetData().(type) {
	case *metricspb.Metric_Gauge:
		base.MetricType = telemetry.MetricTypeGauge
		return n.numberPoints(base, data.Gauge.GetDataPoints(), resource, limit)
	case *metricspb.Metric_Sum:
		if !validTemporality(data.Sum.GetAggregationTemporality()) {
			return nil, metricPointCount(metric)
		}
		base.MetricType = telemetry.MetricTypeSum
		monotonic := data.Sum.GetIsMonotonic()
		base.IsMonotonic = &monotonic
		base.Temporality = temporalityName(data.Sum.GetAggregationTemporality())
		return n.numberPoints(base, data.Sum.GetDataPoints(), resource, limit)
	case *metricspb.Metric_Histogram:
		if !validTemporality(data.Histogram.GetAggregationTemporality()) {
			return nil, metricPointCount(metric)
		}
		base.MetricType = telemetry.MetricTypeHistogram
		base.Temporality = temporalityName(data.Histogram.GetAggregationTemporality())
		return n.histogramPoints(base, data.Histogram.GetDataPoints(), resource, limit)
	case *metricspb.Metric_ExponentialHistogram:
		if !validTemporality(data.ExponentialHistogram.GetAggregationTemporality()) {
			return nil, metricPointCount(metric)
		}
		base.MetricType = telemetry.MetricTypeHistogram
		base.Temporality = temporalityName(data.ExponentialHistogram.GetAggregationTemporality())
		return n.expHistogramPoints(base, data.ExponentialHistogram.GetDataPoints(), resource, limit)
	case *metricspb.Metric_Summary:
		base.MetricType, base.Temporality = telemetry.MetricTypeHistogram, "cumulative"
		return n.summaryPoints(base, data.Summary.GetDataPoints(), resource, limit)
	default:
		return nil, metricPointCount(metric)
	}
}

func validTemporality(value metricspb.AggregationTemporality) bool {
	return value == metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA || value == metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE
}

func (n *normalizer) numberPoints(base telemetry.MetricDataPoint, raw []*metricspb.NumberDataPoint, resource *resourcepb.Resource, limit int) ([]telemetry.MetricDataPoint, int) {
	return collectMetricPoints(raw, limit, func(dp *metricspb.NumberDataPoint) (telemetry.MetricDataPoint, error) {
		point, err := n.metricPoint(base, resource, dp.GetAttributes(), dp.GetStartTimeUnixNano(), dp.GetTimeUnixNano(), dp.GetFlags(), dp.GetExemplars())
		if err != nil {
			return point, err
		}
		var value float64
		switch typed := dp.GetValue().(type) {
		case *metricspb.NumberDataPoint_AsDouble:
			value = typed.AsDouble
		case *metricspb.NumberDataPoint_AsInt:
			value = float64(typed.AsInt)
		default:
			return point, errors.New("metric value is required")
		}
		if !finite(value) || (base.IsMonotonic != nil && *base.IsMonotonic && value < 0) {
			return point, errors.New("metric value is invalid")
		}
		point.Value = &value
		return point, nil
	})
}
