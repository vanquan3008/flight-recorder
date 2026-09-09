package parser

import (
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/security"
)

func parseLEEF(raw string, occurredAt time.Time) (security.Event, bool) {
	position := strings.Index(raw, "LEEF:")
	if position < 0 {
		return security.Event{}, false
	}
	body := raw[position:]
	parts := strings.SplitN(body, "|", 7)
	if len(parts) < 6 {
		return security.Event{}, false
	}
	extension, separator := parts[5], '\t'
	if len(parts) == 7 {
		if len(parts[5]) != 1 {
			return security.Event{}, false
		}
		separator = rune(parts[5][0])
		extension = parts[6]
	}
	fields := parseKeyValues(extension, separator)
	event := security.Event{
		Format: "leef", Kind: first(fields, "eventKind", "cat", "event_type"), Category: first(fields, "cat", "category"), Action: first(fields, "action", "act"), Outcome: first(fields, "outcome", "result"), Severity: first(fields, "sev", "severity"),
		ObserverVendor: parts[1], ObserverProduct: parts[2], HostName: first(fields, "srcHostName", "dstHostName", "host"), UserName: first(fields, "usrName", "user"),
		SourceIP: first(fields, "src", "srcIP"), DestinationIP: first(fields, "dst", "dstIP"), SourcePort: integer(fields, "srcPort", "spt"), DestinationPort: integer(fields, "dstPort", "dpt"),
		NetworkTransport: first(fields, "transport", "proto"), NetworkProtocol: first(fields, "protocol", "applicationProtocol"), RuleIdentifier: parts[4], RuleName: first(fields, "ruleName", "name"), Message: first(fields, "msg", "message"), Attributes: fields, OccurredAt: occurredAt,
	}
	if event.Kind == "" {
		event.Kind = "network"
	}
	return event, true
}
