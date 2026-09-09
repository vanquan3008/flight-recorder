package otlp

import (
	"errors"
	"math"

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

// metrics normalizes an OTLP ExportMetricsServiceRequest into a flat slice of
// MetricDataPoint values ready for persistence. Each OTLP data point becomes
// exactly one row. Returns the data points and the number of rejected records.
func (n *normalizer) metrics(resourceMetrics []*metricspb.ResourceMetrics) ([]telemetry.MetricDataPoint, int) {
	points := make([]telemetry.MetricDataPoint, 0)
	rejected := 0
	sequence := 0
	for _, rm := range resourceMetrics {
		resAttrs, resErr := n.attributeValues(rm.GetResource().GetAttributes())
		if resErr != nil {
			rejected++
			continue
		}
		serviceName := stringAttribute(resAttrs, "service.name")
		serviceVersion := stringAttribute(resAttrs, "service.version")

		for _, sm := range rm.GetScopeMetrics() {
			for _, metric := range sm.GetMetrics() {
				extracted, err := n.extractDataPoints(
					rm.GetResource(), rm.GetSchemaUrl(),
					sm.GetScope(),
					metric,
					serviceName, serviceVersion,
					resAttrs,
				)
				if err != nil {
					rejected += len(extracted) + 1
					continue
				}
				for _, dp := range extracted {
					sequence++
					if sequence > n.configuration.MaxRecordsPerRequest {
						rejected++
						continue
					}
					points = append(points, dp)
				}
			}
		}
	}
	return points, rejected
}

func (n *normalizer) extractDataPoints(
	resource *resourcepb.Resource, _ string,
	scope *commonpb.InstrumentationScope,
	metric *metricspb.Metric,
	serviceName, serviceVersion string,
	resourceAttrs map[string]any,
) ([]telemetry.MetricDataPoint, error) {
	if metric.GetName() == "" {
		return nil, errors.New("metric name is required")
	}
	if textErr := n.validateText(metric.GetName(), metric.GetUnit(), metric.GetDescription()); textErr != nil {
		return nil, textErr
	}

	base := telemetry.MetricDataPoint{
		ProjectID:          n.configuration.ProjectID,
		EnvironmentID:      n.configuration.EnvironmentID,
		MetricName:         metric.GetName(),
		MetricUnit:         metric.GetUnit(),
		ServiceName:        serviceName,
		ServiceVersion:     serviceVersion,
		ResourceAttributes: resourceAttrs,
		ScopeName:          scope.GetName(),
		ScopeVersion:       scope.GetVersion(),
	}

	switch data := metric.Data.(type) {
	case *metricspb.Metric_Gauge:
		return n.gaugePoints(base, data.Gauge.GetDataPoints(), resource)
	case *metricspb.Metric_Sum:
		mono := data.Sum.GetIsMonotonic()
		temporality := temporalityName(data.Sum.GetAggregationTemporality())
		return n.sumPoints(base, data.Sum.GetDataPoints(), resource, mono, temporality)
	case *metricspb.Metric_Histogram:
		temporality := temporalityName(data.Histogram.GetAggregationTemporality())
		return n.histogramPoints(base, data.Histogram.GetDataPoints(), resource, temporality)
	case *metricspb.Metric_ExponentialHistogram:
		// Flatten exponential histogram to a regular histogram using explicit
		// boundaries approximated from the scale and bucket indices.
		temporality := temporalityName(data.ExponentialHistogram.GetAggregationTemporality())
		return n.expHistogramPoints(base, data.ExponentialHistogram.GetDataPoints(), resource, temporality)
	case *metricspb.Metric_Summary:
		// Store summary aggregates in the shared histogram columns.
		return n.summaryPoints(base, data.Summary.GetDataPoints(), resource)
	default:
		return nil, errors.New("unsupported metric instrument type")
	}
}

func (n *normalizer) gaugePoints(
	base telemetry.MetricDataPoint,
	dps []*metricspb.NumberDataPoint,
	resource *resourcepb.Resource,
) ([]telemetry.MetricDataPoint, error) {
	base.MetricType = telemetry.MetricTypeGauge
	return n.numberPoints(base, dps, resource)
}

func (n *normalizer) sumPoints(
	base telemetry.MetricDataPoint,
	dps []*metricspb.NumberDataPoint,
	resource *resourcepb.Resource,
	isMonotonic bool, temporality string,
) ([]telemetry.MetricDataPoint, error) {
	base.MetricType = telemetry.MetricTypeSum
	base.IsMonotonic = &isMonotonic
	base.Temporality = temporality
	return n.numberPoints(base, dps, resource)
}

func (n *normalizer) numberPoints(
	base telemetry.MetricDataPoint,
	dps []*metricspb.NumberDataPoint,
	resource *resourcepb.Resource,
) ([]telemetry.MetricDataPoint, error) {
	points := make([]telemetry.MetricDataPoint, 0, len(dps))
	for _, dp := range dps {
		attrs, attrErr := n.attributeValues(mergeAttributes(resource.GetAttributes(), dp.GetAttributes()))
		if attrErr != nil {
			return points, attrErr
		}
		ts := numberDataPointTimestamp(dp)
		if ts.IsZero() {
			continue
		}
		point := base
		point.OccurredAt = ts
		point.Attributes = attrs

		var v float64
		switch val := dp.Value.(type) {
		case *metricspb.NumberDataPoint_AsDouble:
			if math.IsNaN(val.AsDouble) || math.IsInf(val.AsDouble, 0) {
				continue // drop non-finite values
			}
			v = val.AsDouble
		case *metricspb.NumberDataPoint_AsInt:
			v = float64(val.AsInt)
		default:
			continue
		}
		point.Value = &v
		point.ID = metricPointID(point)
		points = append(points, point)
	}
	return points, nil
}
