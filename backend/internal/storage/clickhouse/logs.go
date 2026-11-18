package clickhouse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
)

func (store *Store) StoreLogs(ctx context.Context, projectID, environmentID string, events []telemetry.Event) error {
	rows := make([]storedLog, 0, len(events))
	for _, event := range events {
		if event.ProjectID != projectID || event.EnvironmentID != environmentID || event.Signal != telemetry.SignalLog {
			return fmt.Errorf("ClickHouse log ingestion scope mismatch: %w", telemetry.ErrInvalidEvent)
		}
		row, err := eventLog(event)
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	return store.insertLogs(ctx, projectID, environmentID, rows)
}

func (store *Store) insertLogs(ctx context.Context, projectID, environmentID string, rows []storedLog) error {
	if err := validateScope(projectID, environmentID); err != nil {
		return err
	}
	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	for _, row := range rows {
		if !validUUID(row.EventID) || row.OccurredAt.IsZero() {
			return fmt.Errorf("invalid log identity or timestamp: %w", telemetry.ErrInvalidEvent)
		}
		if err := encoder.Encode(row); err != nil {
			return fmt.Errorf("encode ClickHouse log: %w", telemetry.ErrInvalidEvent)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	_, err := store.execute(ctx, "INSERT INTO tracepilot_logs FORMAT JSONEachRow", nil, payload.Bytes(), true)
	return err
}

func (store *Store) scope(projectID, environmentID string) ([]string, map[string]string, error) {
	if err := validateScope(projectID, environmentID); err != nil {
		return nil, nil, err
	}
	where := []string{"project_id = {project:UUID}", "environment_id = {environment:UUID}"}
	parameters := map[string]string{"project": projectID, "environment": environmentID}
	return where, parameters, nil
}

// ExpireProjectLogs rewrites the affected parts so cleanup also removes the log payloads from storage.
// Check for matching rows before submitting a mutation on every retention tick.
func (store *Store) ExpireProjectLogs(ctx context.Context, projectID string, before time.Time) error {
	if !validUUID(projectID) || before.IsZero() {
		return errors.New("invalid ClickHouse log retention scope")
	}
	if !before.After(time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)) {
		return nil // DateTime64 cannot represent any earlier log.
	}
	params := map[string]string{"project": projectID}
	where := addTime([]string{"project_id = {project:UUID}"}, params, "occurred_at", "before", "<", before)
	predicate := strings.Join(where, " AND ")
	data, err := store.execute(ctx, "SELECT 1 FROM tracepilot_logs WHERE "+predicate+" LIMIT 1 FORMAT TabSeparated", params, nil, false)
	if err != nil || len(data) == 0 {
		return err
	}
	_, err = store.execute(ctx, "ALTER TABLE tracepilot_logs DELETE WHERE "+predicate+" SETTINGS mutations_sync = 1", params, nil, false)
	return err
}

func addTime(where []string, parameters map[string]string, column, name, comparison string, at time.Time) []string {
	if at.IsZero() {
		return where
	}
	parameters[name] = at.UTC().Format("2006-01-02 15:04:05.999999999")
	return append(where, column+" "+comparison+" {"+name+":DateTime64(9, 'UTC')}")
}

func (store *Store) ListLogs(ctx context.Context, projectID string, filter query.LogFilter) ([]query.LogRecord, error) {
	if filter.Limit < 1 || filter.Limit > 1000 {
		return nil, errors.New("log limit must be between 1 and 1000")
	}
	where, parameters, err := store.scope(projectID, filter.EnvironmentID)
	if err != nil {
		return nil, err
	}
	for _, field := range []struct{ column, name, value string }{
		{"service_name", "service", filter.Service},
		{"trace_id", "trace", filter.TraceID}, {"span_id", "span", filter.SpanID},
	} {
		if field.value != "" {
			where = append(where, field.column+" = {"+field.name+":String}")
			parameters[field.name] = field.value
		}
	}
	if filter.Severity != "" {
		where = append(where, logSeverityPredicate(filter.Severity, parameters))
	}
	if filter.Search != "" {
		where = append(where, "body ILIKE {search:String}")
		parameters["search"] = "%" + filter.Search + "%"
	}
	where = addTime(where, parameters, "occurred_at", "from", ">=", filter.From)
	where = addTime(where, parameters, "occurred_at", "to", "<=", filter.To)
	if !filter.Before.IsZero() {
		where = append(where, "(occurred_at, toString(event_id)) < ({before:DateTime64(9, 'UTC')}, {cursor:String})")
		parameters["before"], parameters["cursor"] = filter.Before.UTC().Format("2006-01-02 15:04:05.999999999"), filter.CursorID
	}
	parameters["limit"] = strconv.Itoa(filter.Limit + 1)
	return store.readLogs(ctx, "SELECT "+logColumns+" FROM tracepilot_logs FINAL WHERE "+strings.Join(where, " AND ")+
		" ORDER BY occurred_at DESC, toString(event_id) DESC LIMIT {limit:UInt32} FORMAT JSONEachRow", parameters)
}

// Standard filters select an OTLP severity group. Text remains untouched in storage.
func logSeverityPredicate(value string, parameters map[string]string) string {
	level := strings.ToUpper(strings.TrimSpace(value))
	switch level {
	case "INFORMATION":
		level = "INFO"
	case "WARNING":
		level = "WARN"
	}
	for index, group := range []string{"TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"} {
		if level != group && !(strings.HasPrefix(level, group) && len(level) == len(group)+1 && level[len(group)] >= '1' && level[len(group)] <= '4') {
			continue
		}
		parameters["severity_min"], parameters["severity_max"] = strconv.Itoa(index*4+1), strconv.Itoa(index*4+4)
		pattern := group + "[1-4]?"
		if group == "INFO" {
			pattern += "|INFORMATION|"
		} else if group == "WARN" {
			pattern += "|WARNING"
		}
		parameters["severity_pattern"] = "^(" + pattern + ")$"
		return "(severity_number BETWEEN {severity_min:Int32} AND {severity_max:Int32} OR " +
			"(NOT (severity_number BETWEEN 1 AND 24) AND match(upperUTF8(trimBoth(severity_text)), {severity_pattern:String})))"
	}
	parameters["severity"] = level
	return "upperUTF8(trimBoth(severity_text)) = {severity:String}"
}

func (store *Store) GetLog(ctx context.Context, projectID, environmentID, eventID string) (query.LogRecord, error) {
	return store.GetLogSince(ctx, projectID, environmentID, eventID, time.Time{})
}

func (store *Store) GetLogSince(ctx context.Context, projectID, environmentID, eventID string, since time.Time) (query.LogRecord, error) {
	if !validUUID(eventID) {
		return query.LogRecord{}, errors.New("invalid log ID")
	}
	where, parameters, err := store.scope(projectID, environmentID)
	if err != nil {
		return query.LogRecord{}, err
	}
	where = append(where, "event_id = {event:UUID}")
	parameters["event"] = eventID
	where = addTime(where, parameters, "occurred_at", "retention_cutoff", ">=", since)
	rows, err := store.readLogs(ctx, "SELECT "+logColumns+" FROM tracepilot_logs FINAL WHERE "+strings.Join(where, " AND ")+
		" LIMIT 1 FORMAT JSONEachRow", parameters)
	if err != nil || len(rows) == 0 {
		return query.LogRecord{}, err
	}
	return rows[0], nil
}

func (store *Store) readLogs(ctx context.Context, sql string, parameters map[string]string) ([]query.LogRecord, error) {
	data, err := store.execute(ctx, sql, parameters, nil, false)
	if err != nil {
		return nil, err
	}
	decoder, records := json.NewDecoder(bytes.NewReader(data)), []query.LogRecord{}
	for {
		var row storedLog
		if err := decoder.Decode(&row); err != nil {
			if errors.Is(err, io.EOF) {
				return records, nil
			}
			return nil, errors.New("decode ClickHouse logs")
		}
		record, err := row.record()
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
}

func (store *Store) LogQuality(ctx context.Context, projectID, environmentID, service string, from, to time.Time) (logs, uncorrelated int64, err error) {
	where, parameters, err := store.scope(projectID, environmentID)
	if err != nil {
		return 0, 0, err
	}
	where = addTime(where, parameters, "occurred_at", "from", ">=", from)
	where = addTime(where, parameters, "occurred_at", "to", "<=", to)
	if service != "" {
		where = append(where, "service_name = {service:String}")
		parameters["service"] = service
	}
	data, err := store.execute(ctx, "SELECT count() AS logs, countIf(trace_id = '') AS uncorrelated FROM tracepilot_logs FINAL WHERE "+
		strings.Join(where, " AND ")+" FORMAT JSONEachRow", parameters, nil, false)
	if err != nil {
		return 0, 0, err
	}
	var result struct{ Logs, Uncorrelated int64 }
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, 0, errors.New("decode ClickHouse log counts")
	}
	return result.Logs, result.Uncorrelated, nil
}
