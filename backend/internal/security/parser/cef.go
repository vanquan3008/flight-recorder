package parser

import (
	"strconv"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/security"
)

func parseCEF(raw string, occurredAt time.Time) (security.Event, bool) {
	position := strings.Index(raw, "CEF:")
	if position < 0 {
		return security.Event{}, false
	}
	parts := splitEscaped(raw[position:], '|', 8)
	if len(parts) != 8 || !strings.HasPrefix(parts[0], "CEF:") {
		return security.Event{}, false
	}
	fields := parseKeyValues(parts[7], ' ')
	event := security.Event{
		Format: "cef", Kind: first(fields, "event_kind", "cat", "deviceEventClassId"), Category: first(fields, "cat", "category"), Action: first(fields, "act", "action"), Outcome: first(fields, "outcome", "result"), Severity: normalizeCEFSeverity(parts[6]),
		ObserverVendor: parts[1], ObserverProduct: parts[2], HostName: first(fields, "shost", "dhost", "host"), UserName: first(fields, "suser", "duser", "user"),
		SourceIP: first(fields, "src", "sourceAddress"), DestinationIP: first(fields, "dst", "destinationAddress"), SourcePort: integer(fields, "spt", "sourcePort"), DestinationPort: integer(fields, "dpt", "destinationPort"),
		NetworkTransport: first(fields, "transport", "proto"), NetworkProtocol: first(fields, "app", "applicationProtocol"), RuleIdentifier: parts[4], RuleName: parts[5], Message: first(fields, "msg", "message"), Attributes: fields, OccurredAt: occurredAt,
	}
	if event.Kind == "" {
		event.Kind = "network"
	}
	return event, true
}

func normalizeCEFSeverity(value string) string {
	level, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return strings.ToUpper(strings.TrimSpace(value))
	}
	switch {
	case level >= 9:
		return "CRITICAL"
	case level >= 7:
		return "HIGH"
	case level >= 4:
		return "MEDIUM"
	default:
		return "LOW"
	}
}
