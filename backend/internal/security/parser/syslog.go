package parser

import (
	"strconv"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/security"
)

func parseSyslog(raw string, occurredAt time.Time) (security.Event, bool) {
	if !strings.HasPrefix(raw, "<") {
		return security.Event{}, false
	}
	endPriority := strings.IndexByte(raw, '>')
	if endPriority < 2 {
		return security.Event{}, false
	}
	priority, priorityError := strconv.Atoi(raw[1:endPriority])
	if priorityError != nil || priority < 0 || priority > 191 {
		return security.Event{}, false
	}
	payload := strings.TrimSpace(raw[endPriority+1:])
	if payload == "" {
		return security.Event{}, false
	}
	if event, ok := parseCEF(payload, occurredAt); ok {
		event.Format = "syslog+cef"
		return event, true
	}
	if event, ok := parseLEEF(payload, occurredAt); ok {
		event.Format = "syslog+leef"
		return event, true
	}
	if event, ok := parseJSON(payload, occurredAt); ok {
		event.Format = "syslog+json"
		return event, true
	}
	words := strings.Fields(payload)
	host := ""
	message := payload
	parsedTime := occurredAt
	// RFC 5424 has seven header fields after the priority: version,
	// timestamp, hostname, app-name, procid, msgid and structured-data.
	// Treat short frames as RFC 3164/plain messages instead of slicing past
	// the end of the token list. These records come from the network.
	if len(words) >= 7 && words[0] == "1" {
		host = words[2]
		message = strings.Join(words[6:], " ")
		if timestamp, parseError := time.Parse(time.RFC3339Nano, words[1]); parseError == nil {
			parsedTime = timestamp.UTC()
		}
	} else if len(words) >= 4 {
		host = words[3]
		message = strings.Join(words[4:], " ")
		if timestamp, parseError := time.ParseInLocation("Jan 2 15:04:05", strings.Join(words[:3], " "), occurredAt.Location()); parseError == nil {
			parsedTime = time.Date(occurredAt.Year(), timestamp.Month(), timestamp.Day(), timestamp.Hour(), timestamp.Minute(), timestamp.Second(), 0, timestamp.Location()).UTC()
		}
	}
	return security.Event{Format: "syslog", Kind: "log", Category: "system", HostName: host, ObserverHostname: host, Message: message, Attributes: map[string]any{"syslog_priority": priority}, OccurredAt: parsedTime}, true
}
