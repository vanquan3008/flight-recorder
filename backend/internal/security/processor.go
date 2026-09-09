// Package security converts recognized raw log records into linked security
// events. It is deliberately called before persistence so raw and normalized
// records share one transaction.
package security

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/security/parser"
)

func Process(events []telemetry.Event) []telemetry.Event {
	for index := range events {
		event := &events[index]
		if event.Signal != telemetry.SignalLog || event.Security != nil {
			continue
		}
		raw, ok := event.LogBody.(string)
		if !ok {
			continue
		}
		parsed, recognized := parser.Parse(raw, event.Timestamp)
		if !recognized {
			continue
		}
		parsed.ID = telemetry.DeterministicID(event.Timestamp, "security-event", event.ID)
		parsed.RawEventID = event.ID
		parsed.SourceID = attributeText(event.Attributes, "log.source.id")
		parsed.SourceName = attributeText(event.Attributes, "log.source.name")
		if parsed.Attributes == nil {
			parsed.Attributes = make(map[string]any)
		}
		parsed.Attributes["raw_format"] = parsed.Format
		if parsed.Message == "" {
			parsed.Message = strings.TrimSpace(raw)
		}
		if parsed.Validate() != nil {
			continue
		}
		event.Security = &parsed
		if event.Attributes == nil {
			event.Attributes = make(map[string]any)
		}
		event.Attributes["tracepilot.security_format"] = parsed.Format
		event.Attributes["tracepilot.security_event_id"] = parsed.ID
		if event.Severity == "" {
			event.Severity = parsed.Severity
		}
	}
	return events
}

func attributeText(attributes map[string]any, key string) string {
	if value, ok := attributes[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

// FromSyslogWithSourceIdentity preserves the peer address and, when mTLS
// supplied one, uses the certificate identity as the stable source name.
func FromSyslogWithSourceIdentity(projectID, environment, remoteAddress, sourceIdentity, raw string, occurredAt time.Time) telemetry.Event {
	sourceName := remoteAddress
	if host, _, splitError := net.SplitHostPort(remoteAddress); splitError == nil {
		sourceName = host
	}
	sourceIdentity = strings.TrimSpace(sourceIdentity)
	if sourceIdentity != "" {
		sourceName = sourceIdentity
	}
	identity := fmt.Sprintf("%s:%s", sourceName, raw)
	sourceID := telemetry.DeterministicID(time.Unix(0, 0), "security-source", projectID, environment, "syslog", sourceName)
	event := telemetry.Event{
		ID:          telemetry.DeterministicID(occurredAt, "syslog", projectID, identity),
		ProjectID:   projectID,
		Environment: environment,
		ServiceName: "syslog",
		Type:        telemetry.EventLog,
		Timestamp:   occurredAt,
		Signal:      telemetry.SignalLog,
		Severity:    "INFO",
		LogBody:     raw,
		Attributes: map[string]any{
			"log.source.type":      "syslog",
			"log.source.id":        sourceID,
			"log.source.name":      sourceName,
			"network.peer.address": remoteAddress,
		},
		ResourceAttributes: map[string]any{
			"service.name": "syslog",
		},
		RecordAttributes: map[string]any{
			"log.source.type": "syslog",
		},
	}
	if sourceIdentity != "" {
		event.Attributes["log.source.identity"] = sourceIdentity
	}
	return Process([]telemetry.Event{event})[0]
}
