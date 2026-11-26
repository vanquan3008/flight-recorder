package telemetryquery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/query"
)

type testCutoff struct {
	value time.Time
	err   error
}

func (cutoff *testCutoff) Cutoff(context.Context, string) (time.Time, error) {
	return cutoff.value, cutoff.err
}

type logProbe struct {
	logReader
	from  time.Time
	calls int
}

func (probe *logProbe) ListLogs(_ context.Context, _ string, filter query.LogFilter) ([]query.LogRecord, error) {
	probe.from = filter.From
	probe.calls++
	return nil, nil
}

func TestListLogsAppliesCutoffBeforeReadingTelemetry(t *testing.T) {
	ctx := context.Background()
	cutoff := &testCutoff{value: time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)}
	logs := &logProbe{}
	service := New(logs, nil, nil, nil, cutoff)
	for _, from := range []time.Time{time.Time{}, cutoff.value.Add(time.Hour)} {
		if _, err := service.ListLogs(ctx, "project", query.LogFilter{From: from}); err != nil {
			t.Fatal(err)
		}
		want := cutoff.value
		if from.After(want) {
			want = from
		}
		if !logs.from.Equal(want) {
			t.Fatalf("log lower bound = %v, want %v", logs.from, want)
		}
	}
	cutoff.err = errors.New("policy unavailable")
	if _, err := service.ListLogs(ctx, "project", query.LogFilter{}); !errors.Is(err, cutoff.err) || logs.calls != 2 {
		t.Fatalf("failed policy lookup = %v with %d telemetry reads, want %v and only the previous two reads", err, logs.calls, cutoff.err)
	}
}
