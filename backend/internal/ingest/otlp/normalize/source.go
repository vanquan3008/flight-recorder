package normalize

import (
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

// source is what a span or a log record inherits from where it came from: its resource, its
// instrumentation scope, and its own attributes, each already converted and limited.
type source struct {
	resourceAttributes map[string]any
	scopeAttributes    map[string]any
	recordAttributes   map[string]any
	resourceSchemaURL  string
	scope              *commonpb.InstrumentationScope
	scopeSchemaURL     string
}

func (normalizer *Normalizer) source(resource *resourcepb.Resource, resourceSchemaURL string, scope *commonpb.InstrumentationScope, scopeSchemaURL string, record []*commonpb.KeyValue) (source, error) {
	resourceAttributes, resourceError := normalizer.attributeValues(resource.GetAttributes())
	if resourceError != nil {
		return source{}, resourceError
	}
	scopeAttributes, scopeError := normalizer.attributeValues(scope.GetAttributes())
	if scopeError != nil {
		return source{}, scopeError
	}
	recordAttributes, recordError := normalizer.attributeValues(record)
	if recordError != nil {
		return source{}, recordError
	}
	return source{
		resourceAttributes: resourceAttributes, scopeAttributes: scopeAttributes, recordAttributes: recordAttributes,
		resourceSchemaURL: resourceSchemaURL, scope: scope, scopeSchemaURL: scopeSchemaURL,
	}, nil
}

// stamp fills the fields every signal shares: the configured project and environment, the service and
// request the attributes name, and the resource and scope the record came from.
func (normalizer *Normalizer) stamp(event *telemetry.Event, from source, attributes map[string]any) {
	event.ProjectID = normalizer.configuration.ProjectID
	event.EnvironmentID = normalizer.configuration.EnvironmentID
	event.Environment = normalizer.configuration.EnvironmentName
	event.ServiceName = stringAttribute(attributes, "service.name")
	event.ServiceVersion = stringAttribute(attributes, "service.version")
	event.RequestID = requestID(attributes)
	event.Attributes = attributes
	event.ResourceAttributes = from.resourceAttributes
	event.ResourceSchemaURL = from.resourceSchemaURL
	event.ScopeName = from.scope.GetName()
	event.ScopeVersion = from.scope.GetVersion()
	event.ScopeSchemaURL = from.scopeSchemaURL
	event.ScopeAttributes = from.scopeAttributes
	event.RecordAttributes = from.recordAttributes
}
