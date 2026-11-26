// Package normalizetest builds the configuration and OTLP values the ingest tests share.
package normalizetest

import (
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry/telemetrytest"
)

// Config is a small ingestion scope with generous limits.
func Config() config.IngestionConfig {
	return config.IngestionConfig{
		ProjectID:              telemetrytest.Project,
		EnvironmentID:          telemetrytest.Environment,
		EnvironmentName:        "development",
		MaxRecordsPerRequest:   100,
		MaxAttributesPerRecord: 16,
		MaxAttributeValueBytes: 1024,
		MaxAttributeDepth:      4,
	}
}

func StringKeyValue(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}}
}

func IntKeyValue(key string, value int64) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: value}}}
}
