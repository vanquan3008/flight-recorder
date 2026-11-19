package clickhouse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
)

// telemetryTable is a table retention expires; the span rollups expire with the spans they summarize.
type telemetryTable struct {
	name, column string
	partition    config.LogPartition
}

func (store *Store) telemetryTables() []telemetryTable {
	return []telemetryTable{
		{"tracepilot_log_records", "occurred_at", store.cfg.LogPartition},
		{"tracepilot_spans", "started_at", config.LogPartitionWeekly},
		{"tracepilot_trace_index", "started_at", config.LogPartitionWeekly},
		{"tracepilot_metric_points", "occurred_at", config.LogPartitionWeekly},
		{"tracepilot_span_minutes", "minute", config.LogPartitionWeekly},
		{"tracepilot_span_minute_marks", "minute", config.LogPartitionWeekly},
		{"tracepilot_span_minute_done", "minute", config.LogPartitionWeekly},
	}
}

// ExpireProjectTelemetry removes one project's rows before the start of the
// cutoff's UTC day with lightweight deletes, which mark rows instead of
// rewriting parts. Partitions every project has expired go through
// DropPartitionsBefore instead.
func (store *Store) ExpireProjectTelemetry(ctx context.Context, projectID string, before time.Time) error {
	if !validUUID(projectID) || before.IsZero() {
		return errors.New("invalid ClickHouse retention scope")
	}
	cutoff := startOfUTCDay(before)
	if !cutoff.After(time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)) {
		return nil // DateTime64 cannot represent any earlier row.
	}
	for _, table := range store.telemetryTables() {
		params := map[string]string{"project": projectID}
		predicate := strings.Join(addTime([]string{"project_id = {project:UUID}"}, params, table.column, "before", "<", cutoff), " AND ")
		data, err := store.execute(ctx, "SELECT 1 FROM "+table.name+" WHERE "+predicate+" LIMIT 1 FORMAT TabSeparated", params, nil, false)
		if err != nil {
			return err
		}
		if len(data) == 0 {
			continue
		}
		if _, err := store.execute(ctx, "DELETE FROM "+table.name+" WHERE "+predicate, params, nil, false); err != nil {
			return err
		}
	}
	return nil
}

// DropPartitionsBefore drops every partition whose time range ends at or before
// the cutoff, which callers derive from the longest project policy.
func (store *Store) DropPartitionsBefore(ctx context.Context, before time.Time) error {
	for _, table := range store.telemetryTables() {
		data, err := store.execute(ctx, "SELECT DISTINCT partition_id FROM system.parts WHERE database = {database:String} AND table = {table:String} AND active FORMAT JSONEachRow",
			map[string]string{"database": store.cfg.Database, "table": table.name}, nil, false)
		if err != nil {
			return err
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		for {
			var part struct {
				ID string `json:"partition_id"`
			}
			if err := decoder.Decode(&part); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return errors.New("decode ClickHouse partitions")
			}
			if end, known := partitionEnd(table.partition, part.ID); !known || end.After(before) {
				continue
			}
			if _, err := store.execute(ctx, "ALTER TABLE "+table.name+" DROP PARTITION ID '"+part.ID+"'", nil, nil, false); err != nil {
				return err
			}
		}
	}
	return nil
}

// partitionEnd reads the end of a partition's time range from its ID: YYYYMMDD
// for a day or a week's Monday, YYYYMM for a month and YYYY for a year. Only
// digit IDs are accepted, so an ID is safe to place in DROP PARTITION.
func partitionEnd(granularity config.LogPartition, id string) (time.Time, bool) {
	layout, days, months, years := "20060102", 7, 0, 0
	switch granularity {
	case config.LogPartitionDaily:
		days = 1
	case config.LogPartitionMonthly:
		layout, days, months = "200601", 0, 1
	case config.LogPartitionYearly:
		layout, days, years = "2006", 0, 1
	}
	if len(id) != len(layout) || strings.Trim(id, "0123456789") != "" {
		return time.Time{}, false
	}
	start, err := time.ParseInLocation(layout, id, time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	return start.AddDate(years, months, days), true
}

func startOfUTCDay(at time.Time) time.Time {
	at = at.UTC()
	return time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
}
