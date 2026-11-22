package clickhouse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	_, err := store.execute(ctx, "INSERT INTO tracepilot_log_records FORMAT JSONEachRow", nil, payload.Bytes(), true)
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

// finalSettings is safe because a retried log lands in the partition of its
// first copy: the partition key derives from occurred_at, a deduplication key.
const finalSettings = " SETTINGS do_not_merge_across_partitions_select_final = 1"

// ListLogs returns Limit+1 rows to signal another page. It reads keys first,
// then full rows within the page's time range, so bodies are read for one page.
func (store *Store) ListLogs(ctx context.Context, projectID string, filter query.LogFilter) ([]query.LogRecord, error) {
	if filter.Limit < 1 || filter.Limit > 1000 {
		return nil, errors.New("log limit must be between 1 and 1000")
	}
	where, parameters, err := store.logFilter(projectID, filter)
	if err != nil {
		return nil, err
	}
	parameters["limit"] = strconv.Itoa(filter.Limit + 1)
	keys, err := store.readLogKeys(ctx, "SELECT occurred_at, toString(event_id) AS id FROM tracepilot_log_records FINAL WHERE "+
		strings.Join(where, " AND ")+" ORDER BY occurred_at DESC, id DESC LIMIT {limit:UInt32}"+finalSettings+" FORMAT JSONEachRow", parameters)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return []query.LogRecord{}, nil
	}
	page := filter
	page.From, page.To = keys[len(keys)-1].OccurredAt, keys[0].OccurredAt
	where, parameters, err = store.logFilter(projectID, page)
	if err != nil {
		return nil, err
	}
	parameters["limit"] = strconv.Itoa(len(keys))
	return store.readLogs(ctx, "SELECT "+logColumns+" FROM tracepilot_log_records FINAL WHERE "+strings.Join(where, " AND ")+
		" ORDER BY occurred_at DESC, toString(event_id) DESC LIMIT {limit:UInt32}"+finalSettings+" FORMAT JSONEachRow", parameters)
}

type logKey struct {
	OccurredAt time.Time `json:"occurred_at"`
	ID         string    `json:"id"`
}

func (store *Store) readLogKeys(ctx context.Context, sql string, parameters map[string]string) ([]logKey, error) {
	data, err := store.execute(ctx, sql, parameters, nil, false)
	if err != nil {
		return nil, err
	}
	keys := []logKey{}
	for key, err := range decodeJSONRows[logKey](bytes.NewReader(data)) {
		if err != nil || key.OccurredAt.IsZero() || !validUUID(key.ID) {
			return nil, errors.New("decode ClickHouse logs")
		}
		keys = append(keys, *key)
	}
	return keys, nil
}

func (store *Store) GetLog(ctx context.Context, projectID, environmentID, eventID string) (query.LogRecord, error) {
	return store.GetLogSince(ctx, projectID, environmentID, eventID, time.Time{})
}

// GetLogSince returns an empty record and a nil error when the log is missing.
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
	rows, err := store.readLogs(ctx, "SELECT "+logColumns+" FROM tracepilot_log_records FINAL WHERE "+strings.Join(where, " AND ")+
		" LIMIT 1"+finalSettings+" FORMAT JSONEachRow", parameters)
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
	records := []query.LogRecord{}
	for row, err := range decodeJSONRows[storedLog](bytes.NewReader(data)) {
		if err != nil {
			return nil, errors.New("decode ClickHouse logs")
		}
		record, err := row.record()
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (store *Store) LogQuality(ctx context.Context, projectID, environmentID, service string, from, to time.Time) (logs, uncorrelated int64, err error) {
	where, parameters, err := store.scope(projectID, environmentID)
	if err != nil {
		return 0, 0, err
	}
	where = addTime(where, parameters, "occurred_at", "from", ">=", from)
	where = addTime(where, parameters, "occurred_at", "to", "<=", to)
	where = addEquals(where, parameters, [3]string{"service_name", "service", service})
	data, err := store.execute(ctx, "SELECT count() AS logs, countIf(trace_id = '') AS uncorrelated FROM tracepilot_log_records FINAL WHERE "+
		strings.Join(where, " AND ")+finalSettings+" FORMAT JSONEachRow", parameters, nil, false)
	if err != nil {
		return 0, 0, err
	}
	var result struct{ Logs, Uncorrelated int64 }
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, 0, errors.New("decode ClickHouse log counts")
	}
	return result.Logs, result.Uncorrelated, nil
}
