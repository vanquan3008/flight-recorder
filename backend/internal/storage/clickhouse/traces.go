package clickhouse

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/query"
)

// traceSummaryLookaround bounds the summary of a page's traces to their spans
// that started within an hour of the matching window.
const traceSummaryLookaround = time.Hour

type traceSummaryRow struct {
	TraceID       string    `json:"id"`
	StartedAt     time.Time `json:"trace_started"`
	EndedAt       time.Time `json:"trace_ended"`
	CursorAt      time.Time `json:"cursor_at"`
	DurationNanos int64     `json:"duration_nanos"`
	RootService   string    `json:"root_service"`
	SpanCount     int       `json:"span_count"`
	ErrorCount    int       `json:"error_count"`
}

type tracePageRow struct {
	TraceID   string    `json:"trace_id"`
	LastMatch time.Time `json:"last_match"`
}

const traceSummaryColumns = `s.trace_id AS id, min(s.started_at) AS trace_started, max(s.ended_at) AS trace_ended,
toInt64(toUnixTimestamp64Nano(max(s.ended_at)) - toUnixTimestamp64Nano(min(s.started_at))) AS duration_nanos,
argMin(s.service_name, tuple(s.parent_span_id != '', s.started_at, toString(s.event_id))) AS root_service,
count() AS span_count, countIf(s.status_code = 'ERROR') AS error_count`

// ListTraces returns up to Limit+1 traces ordered by their latest matching span.
// Without a duration filter the page is chosen from matching spans first and
// only its traces are summarized; a duration filter needs every match summarized.
func (store *Store) ListTraces(ctx context.Context, projectID string, filter query.TraceFilter) ([]query.TraceSummary, error) {
	if filter.Limit < 1 || filter.Limit > 1000 {
		return nil, errors.New("trace limit must be between 1 and 1000")
	}
	where, parameters, err := store.scope(projectID, filter.EnvironmentID)
	if err != nil {
		return nil, err
	}
	matching := addTime(append([]string{}, where...), parameters, "started_at", "from", ">=", maxTime(filter.From, filter.Since))
	matching = addTime(matching, parameters, "started_at", "to", "<=", filter.To)
	matching = addEquals(matching, parameters, [3]string{"service_name", "service", filter.Service},
		[3]string{"span_name", "operation", filter.Operation}, [3]string{"status_code", "status", filter.Status},
		[3]string{"trace_id", "trace", filter.TraceID})
	page := "SELECT trace_id, max(started_at) AS last_match FROM tracepilot_spans FINAL WHERE " + strings.Join(matching, " AND ") + " GROUP BY trace_id"
	if !filter.Before.IsZero() {
		page += " HAVING (last_match, trace_id) < ({before:DateTime64(9, 'UTC')}, {cursor:String})"
		parameters["before"], parameters["cursor"] = chTime(filter.Before), filter.CursorID
	}
	parameters["limit"] = strconv.Itoa(filter.Limit + 1)
	summaryWhere := []string{"s.project_id = {project:UUID}", "s.environment_id = {environment:UUID}"}
	summaryFrom := filter.Since
	if !filter.From.IsZero() {
		summaryFrom = maxTime(summaryFrom, filter.From.Add(-traceSummaryLookaround))
	}
	summaryWhere = addTime(summaryWhere, parameters, "s.started_at", "summary_from", ">=", summaryFrom)
	if !filter.To.IsZero() {
		summaryWhere = addTime(summaryWhere, parameters, "s.started_at", "summary_to", "<=", filter.To.Add(traceSummaryLookaround))
	}
	if filter.MinDurationNs == nil && filter.MaxDurationNs == nil {
		return store.summarizeTracePage(ctx, page+" ORDER BY last_match DESC, trace_id DESC LIMIT {limit:UInt32}", summaryWhere, parameters)
	}
	having := []string{}
	if filter.MinDurationNs != nil {
		having = append(having, "duration_nanos >= {min_duration:Int64}")
		parameters["min_duration"] = strconv.FormatInt(*filter.MinDurationNs, 10)
	}
	if filter.MaxDurationNs != nil {
		having = append(having, "duration_nanos <= {max_duration:Int64}")
		parameters["max_duration"] = strconv.FormatInt(*filter.MaxDurationNs, 10)
	}
	rows, err := queryRows[traceSummaryRow](ctx, store, "SELECT "+traceSummaryColumns+", any(p.last_match) AS cursor_at FROM tracepilot_spans AS s FINAL INNER JOIN ("+page+
		") AS p ON s.trace_id = p.trace_id WHERE "+strings.Join(summaryWhere, " AND ")+" GROUP BY s.trace_id HAVING "+strings.Join(having, " AND ")+
		" ORDER BY cursor_at DESC, id DESC LIMIT {limit:UInt32}"+finalSettings+" FORMAT JSONEachRow", parameters)
	if err != nil {
		return nil, err
	}
	return traceSummaries(rows), nil
}

func (store *Store) summarizeTracePage(ctx context.Context, page string, summaryWhere []string, parameters map[string]string) ([]query.TraceSummary, error) {
	matches, err := queryRows[tracePageRow](ctx, store, page+finalSettings+" FORMAT JSONEachRow", parameters)
	if err != nil || len(matches) == 0 {
		return []query.TraceSummary{}, err
	}
	ids := make([]string, len(matches))
	lastMatch := make(map[string]time.Time, len(matches))
	for index, match := range matches {
		ids[index], lastMatch[match.TraceID] = match.TraceID, match.LastMatch
	}
	summaryWhere = append(summaryWhere, inStrings("s.trace_id", "trace_", ids, parameters))
	rows, err := queryRows[traceSummaryRow](ctx, store, "SELECT "+traceSummaryColumns+" FROM tracepilot_spans AS s FINAL WHERE "+
		strings.Join(summaryWhere, " AND ")+" GROUP BY s.trace_id"+finalSettings+" FORMAT JSONEachRow", parameters)
	if err != nil {
		return nil, err
	}
	for index := range rows {
		rows[index].CursorAt = lastMatch[rows[index].TraceID]
	}
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].CursorAt.Equal(rows[j].CursorAt) {
			return rows[i].CursorAt.After(rows[j].CursorAt)
		}
		return rows[i].TraceID > rows[j].TraceID
	})
	return traceSummaries(rows), nil
}

func traceSummaries(rows []traceSummaryRow) []query.TraceSummary {
	traces := make([]query.TraceSummary, 0, len(rows))
	for _, row := range rows {
		traces = append(traces, query.TraceSummary{
			TraceID: row.TraceID, StartedAt: row.StartedAt, EndedAt: row.EndedAt, CursorAt: row.CursorAt,
			DurationNanos: row.DurationNanos, RootService: row.RootService, SpanCount: row.SpanCount, ErrorCount: row.ErrorCount,
		})
	}
	return traces
}

type traceBounds struct {
	First time.Time `json:"first_started"`
	Last  time.Time `json:"last_ended"`
}

// GetTrace returns one page of a trace's spans, oldest first, and whether more
// follow. The trace index bounds the read to the trace's own time range.
func (store *Store) GetTrace(ctx context.Context, projectID, environmentID, traceID string, page query.SpanPage) ([]query.SpanRecord, bool, error) {
	where, parameters, err := store.scope(projectID, environmentID)
	if err != nil {
		return nil, false, err
	}
	where = append(where, "trace_id = {trace:String}")
	parameters["trace"] = traceID
	bounds, err := queryRows[traceBounds](ctx, store, "SELECT min(started_at) AS first_started, max(ended_at) AS last_ended FROM tracepilot_trace_index WHERE "+
		strings.Join(where, " AND ")+" HAVING count() > 0 FORMAT JSONEachRow", parameters)
	if err != nil || len(bounds) == 0 {
		return nil, false, err
	}
	where = addTime(where, parameters, "started_at", "first", ">=", maxTime(bounds[0].First, page.Since))
	where = addTime(where, parameters, "started_at", "last", "<=", bounds[0].Last)
	if !page.AfterStartedAt.IsZero() {
		where = append(where, "(started_at, toString(event_id)) > ({after:DateTime64(9, 'UTC')}, {after_id:String})")
		parameters["after"], parameters["after_id"] = chTime(page.AfterStartedAt), page.AfterEventID
	}
	parameters["limit"] = strconv.Itoa(page.Limit + 1)
	response, err := store.openResponse(ctx, "SELECT "+spanColumns+" FROM tracepilot_spans FINAL WHERE "+strings.Join(where, " AND ")+
		" ORDER BY started_at, toString(event_id) LIMIT {limit:UInt32}"+finalSettings+" FORMAT JSONEachRow", parameters, nil, false)
	if err != nil {
		return nil, false, err
	}
	defer response.Close()
	return readSpanPage(ctx, response, page)
}

func readSpanPage(ctx context.Context, body io.Reader, page query.SpanPage) ([]query.SpanRecord, bool, error) {
	// Check only the next row to decide whether a cursor is needed. Cap wire bytes
	// independently of the smaller budget for attributes in the returned page.
	input := &io.LimitedReader{R: body, N: maxResponseBytes + 1}
	decoder := json.NewDecoder(input)
	spans := make([]query.SpanRecord, 0, page.Limit)
	var size int64
	for {
		var row storedSpan
		err := decoder.Decode(&row)
		if decoder.InputOffset() > maxResponseBytes || (err != nil && input.N == 0) {
			return nil, false, errors.New("ClickHouse result exceeds 32 MiB; narrow the span query")
		}
		if errors.Is(err, io.EOF) {
			return spans, false, nil
		}
		if err != nil {
			return nil, false, responseError(ctx, err, "read ClickHouse response")
		}
		if len(spans) == page.Limit {
			return spans, true, nil
		}
		record, err := row.record()
		if err != nil {
			return nil, false, err
		}
		size += int64(len(record.ResourceAttributes) + len(record.ScopeAttributes) + len(record.Attributes) + len(record.Events) + len(record.Links))
		if len(spans) > 0 && page.MaxBytes > 0 && size > page.MaxBytes {
			return spans, true, nil
		}
		spans = append(spans, record)
	}
}

func (row storedSpan) record() (query.SpanRecord, error) {
	if !json.Valid([]byte(row.SpanEvents)) || !json.Valid([]byte(row.SpanLinks)) {
		return query.SpanRecord{}, errors.New("invalid JSON in stored span")
	}
	return query.SpanRecord{
		EventID: row.EventID, TraceID: row.TraceID, SpanID: row.SpanID, ParentSpanID: row.ParentSpanID, TraceState: row.TraceState,
		Name: row.SpanName, Kind: row.SpanKind, StartedAt: row.StartedAt, EndedAt: row.EndedAt, DurationNanos: row.DurationNanos,
		StatusCode: row.StatusCode, StatusMessage: row.StatusMessage, Flags: int64(row.Flags),
		ServiceName: row.ServiceName, ServiceVersion: row.ServiceVersion, ResourceSchemaURL: row.ResourceSchemaURL,
		ResourceAttributes: attributeJSON(row.ResourceAttributes), ScopeName: row.ScopeName, ScopeVersion: row.ScopeVersion,
		ScopeSchemaURL: row.ScopeSchemaURL, ScopeAttributes: attributeJSON(row.ScopeAttributes), Attributes: attributeJSON(row.SpanAttributes),
		Events: json.RawMessage(row.SpanEvents), Links: json.RawMessage(row.SpanLinks),
		DroppedAttributesCount: row.DroppedAttributesCount, DroppedEventsCount: row.DroppedEventsCount, DroppedLinksCount: row.DroppedLinksCount,
	}, nil
}
