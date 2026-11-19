// Package logs coordinates telemetry writes with the incident workflow.
package logs

import (
	"context"

	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

type WriteEvents func(context.Context, []telemetry.Event) error

// Persist writes logs and spans to the telemetry store, then the failures among
// them to the incident store, and acknowledges only after every write succeeds.
// A retry may repeat an already committed write; stores deduplicate by event ID.
func Persist(ctx context.Context, events []telemetry.Event, writeLogs, writeSpans, writeFailures WriteEvents) error {
	var logs, spans, failures []telemetry.Event
	for _, event := range events {
		switch event.Signal {
		case telemetry.SignalLog:
			logs = append(logs, event)
		case telemetry.SignalTrace:
			spans = append(spans, event)
		default:
			continue
		}
		if incident.Candidate(event) {
			failures = append(failures, event)
		}
	}
	for _, write := range []struct {
		events []telemetry.Event
		store  WriteEvents
	}{{logs, writeLogs}, {spans, writeSpans}, {failures, writeFailures}} {
		if len(write.events) == 0 {
			continue
		}
		if err := write.store(ctx, write.events); err != nil {
			return err
		}
	}
	return nil
}
