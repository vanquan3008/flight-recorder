// Package parser normalizes common security log encodings without retaining
// vendor-specific parsing dependencies in the ingest path.
package parser

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/security"
)

// Parse recognizes CEF, LEEF, JSON and Syslog (RFC 3164/5424) wrappers.
// It returns false for regular application logs so OTLP/file ingestion stays
// telemetry-only unless an operator sends a known security format.
func Parse(raw string, occurredAt time.Time) (security.Event, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return security.Event{}, false
	}
	if event, ok := parseSyslog(raw, occurredAt); ok {
		return event, true
	}
	if event, ok := parseCEF(raw, occurredAt); ok {
		return event, true
	}
	if event, ok := parseLEEF(raw, occurredAt); ok {
		return event, true
	}
	if event, ok := parseJSON(raw, occurredAt); ok {
		return event, true
	}
	return security.Event{}, false
}

func parseJSON(raw string, occurredAt time.Time) (security.Event, bool) {
	var values map[string]any
	if json.Unmarshal([]byte(raw), &values) != nil {
		return security.Event{}, false
	}
	value := func(keys ...string) string {
		for _, key := range keys {
			if candidate, ok := values[key]; ok {
				if text, ok := candidate.(string); ok {
					return strings.TrimSpace(text)
				}
			}
		}
		return ""
	}
	kind := value("event_kind", "security_event_type", "event_type")
	if kind == "" {
		// A generic application log is often a JSON object too. Require an
		// explicit kind plus at least one security-shaped field before treating
		// it as a normalized security event.
		kind = value("kind")
		if kind == "" || !hasSecurityField(values) {
			return security.Event{}, false
		}
	}
	event := security.Event{
		Format: "json", Kind: kind, Category: value("category", "event_category"), Action: value("action", "act"), Outcome: value("outcome", "result"), Severity: value("severity", "level"),
		ObserverVendor: value("vendor", "observer_vendor"), ObserverProduct: value("product", "observer_product"), ObserverHostname: value("observer_hostname", "device_hostname"),
		HostName: value("host", "hostname", "host_name"), UserName: value("user", "username", "user_name"), SourceIP: value("src", "source_ip", "src_ip"), DestinationIP: value("dst", "destination_ip", "dst_ip"),
		NetworkTransport: value("transport", "proto"), NetworkProtocol: value("protocol", "application_protocol"), RuleIdentifier: value("rule_id", "signature_id"), RuleName: value("rule_name", "signature"), Message: value("message", "msg"), Attributes: values,
		OccurredAt: occurredAt,
	}
	event.SourcePort = integer(values, "spt", "source_port", "src_port")
	event.DestinationPort = integer(values, "dpt", "destination_port", "dst_port")
	return event, true
}

func hasSecurityField(values map[string]any) bool {
	for _, key := range []string{
		"category", "action", "outcome", "src", "source_ip", "src_ip",
		"dst", "destination_ip", "dst_ip", "signature_id", "rule_id",
		"vendor", "product", "observer_vendor", "observer_product",
	} {
		if candidate, ok := values[key]; ok {
			if text, ok := candidate.(string); ok && strings.TrimSpace(text) != "" {
				return true
			}
		}
	}
	return false
}
