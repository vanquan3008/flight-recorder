package otlp

import (
	"encoding/hex"
	"errors"
	"math"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

func (n *normalizer) metricExemplars(values []*metricspb.Exemplar, fallback time.Time) ([]telemetry.MetricExemplar, error) {
	exemplars := make([]telemetry.MetricExemplar, 0, len(values))
	for _, value := range values {
		if !validIdentifier(value.GetTraceId(), 16, true) || !validIdentifier(value.GetSpanId(), 8, true) {
			return nil, errors.New("metric exemplar identity is invalid")
		}
		attributes, err := n.attributeValues(value.GetFilteredAttributes())
		if err != nil {
			return nil, err
		}
		occurredAt := metricTimestamp(value.GetTimeUnixNano())
		if occurredAt.IsZero() {
			occurredAt = fallback
		}
		var number float64
		switch typed := value.Value.(type) {
		case *metricspb.Exemplar_AsDouble:
			number = typed.AsDouble
		case *metricspb.Exemplar_AsInt:
			number = float64(typed.AsInt)
		default:
			continue
		}
		if math.IsNaN(number) || math.IsInf(number, 0) {
			continue
		}
		exemplars = append(exemplars, telemetry.MetricExemplar{
			OccurredAt: occurredAt,
			Value:      number,
			TraceID:    hex.EncodeToString(value.GetTraceId()),
			SpanID:     hex.EncodeToString(value.GetSpanId()),
			Attributes: attributes,
		})
	}
	return exemplars, nil
}
