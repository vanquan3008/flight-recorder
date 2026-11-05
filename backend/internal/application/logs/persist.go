// Package logs coordinates durable log writes with the existing incident workflow.
package logs

import (
	"context"

	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

type WriteEvents func(context.Context, []telemetry.Event) error

// Persist acknowledges only after every participating store succeeds. A retry
// may repeat an already committed log write; stores must deduplicate by event ID.
func Persist(ctx context.Context, events []telemetry.Event, writeLogs, writeOther, writeFailures WriteEvents) error {
	var logs, other, failures []telemetry.Event
	for _, event := range events {
		if event.Signal != telemetry.SignalLog {
			other = append(other, event)
			continue
		}
		logs = append(logs, event)
		if incident.Candidate(event) {
			failures = append(failures, event)
		}
	}
	if len(logs) > 0 {
		if err := writeLogs(ctx, logs); err != nil {
			return err
		}
	}
	if len(other) > 0 {
		if err := writeOther(ctx, other); err != nil {
			return err
		}
	}
	if len(failures) > 0 {
		return writeFailures(ctx, failures)
	}
	return nil
}
