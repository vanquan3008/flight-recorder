package explorer

import (
	"context"
	"fmt"
	"time"

	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// An open trace window is sent as these outer bounds rather than NULL, so the
// query always compares started_at directly and keeps its index and partition
// pruning. Only a trace ID lookup leaves the start open.
var (
	openWindowStart = time.Unix(0, 0).UTC()
	openWindowEnd   = time.Date(9999, time.December, 31, 0, 0, 0, 0, time.UTC)
)

func traceWindow(from, to time.Time) (time.Time, time.Time) {
	if from.IsZero() {
		from = openWindowStart
	}
	if to.IsZero() {
		to = openWindowEnd
	}
	return from, to
}

func (store *QueryStore) ListTraces(ctx context.Context, projectID string, filter query.TraceFilter) ([]query.TraceSummary, error) {
	from, to := traceWindow(filter.From, filter.To)
	rows, queryError := store.queries.ListTraces(ctx, sqlcstore.ListTracesParams{
		ProjectID:     projectID,
		EnvironmentID: filter.EnvironmentID,
		Service:       pgvalue.OrNil(filter.Service),
		Operation:     pgvalue.OrNil(filter.Operation),
		Status:        pgvalue.OrNil(filter.Status),
		TraceID:       pgvalue.OrNil(filter.TraceID),
		FromTime:      from,
		ToTime:        to,
		Before:        pgvalue.TimeOrNil(filter.Before),
		CursorID:      pgvalue.OrNil(filter.CursorID),
		MinDurationNs: filter.MinDurationNs,
		MaxDurationNs: filter.MaxDurationNs,
		RowLimit:      int32(filter.Limit + 1),
	})
	if queryError != nil {
		return nil, fmt.Errorf("list traces: %w", queryError)
	}
	traces := make([]query.TraceSummary, 0, len(rows))
	for _, row := range rows {
		traces = append(traces, query.TraceSummary{
			TraceID: row.TraceID, StartedAt: row.StartedAt, EndedAt: row.EndedAt, CursorAt: row.CursorAt,
			DurationNanos: row.DurationNanos, RootService: row.RootService,
			SpanCount: int(row.SpanCount), ErrorCount: int(row.ErrorCount),
		})
	}
	return traces, nil
}

// GetTrace returns one page of a trace's spans and whether more follow it. The
// page holds at most page.Limit spans, fewer when page.MaxBytes is reached, so
// a large trace is read in bounded steps instead of being loaded whole.
func (store *QueryStore) GetTrace(ctx context.Context, projectID, environmentID, traceID string, page query.SpanPage) ([]query.SpanRecord, bool, error) {
	rows, queryError := store.queries.ListTraceSpans(ctx, sqlcstore.ListTraceSpansParams{
		ProjectID:      projectID,
		EnvironmentID:  environmentID,
		TraceID:        traceID,
		AfterStartedAt: pgvalue.TimeOrNil(page.AfterStartedAt),
		AfterEventID:   pgvalue.OrNil(page.AfterEventID),
		RowLimit:       int32(page.Limit + 1),
		MaxBytes:       page.MaxBytes,
	})
	if queryError != nil {
		return nil, false, fmt.Errorf("get trace: %w", queryError)
	}
	// The query fetched one row beyond the limit; `fetched` counts them before
	// the byte budget trimmed the page, so a trimmed page still reports more.
	kept := min(len(rows), page.Limit)
	hasMore := len(rows) > 0 && int(rows[0].Fetched) > kept
	rows = rows[:kept]
	spans := make([]query.SpanRecord, 0, len(rows))
	for _, row := range rows {
		spans = append(spans, query.SpanRecord{
			EventID:            row.EventID,
			TraceID:            row.TraceID,
			SpanID:             row.SpanID,
			ParentSpanID:       row.ParentSpanID,
			TraceState:         row.TraceState,
			Name:               row.SpanName,
			Kind:               row.SpanKind,
			StartedAt:          row.StartedAt,
			EndedAt:            row.EndedAt,
			StatusCode:         row.StatusCode,
			StatusMessage:      row.StatusMessage,
			Flags:              row.Flags,
			ServiceName:        row.ServiceName,
			ServiceVersion:     row.ServiceVersion,
			ResourceAttributes: row.ResourceAttributes,
			ScopeName:          row.ScopeName,
			ScopeVersion:       row.ScopeVersion,
			ScopeAttributes:    row.ScopeAttributes,
			Attributes:         row.SpanAttributes,
			Events:             row.SpanEvents,
			Links:              row.SpanLinks,
		})
	}
	return spans, hasMore, nil
}
