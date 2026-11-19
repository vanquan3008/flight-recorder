package clickhouse

import (
	"context"
	"fmt"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func (store *Store) Ready(ctx context.Context) error {
	state, err := store.MigrationStatus(ctx)
	if err != nil {
		return err
	}
	if state.Dirty || state.Version != state.Target {
		return fmt.Errorf("ClickHouse schema version is %d (dirty=%t), expected %d; inspect migrate clickhouse status or run migrate clickhouse up", state.Version, state.Dirty, state.Target)
	}
	return store.readySchema(ctx)
}

// readySchema reads no rows but fails when a column that writes and reads use is missing.
func (store *Store) readySchema(ctx context.Context) error {
	for _, check := range []string{
		"SELECT " + logColumns + " FROM tracepilot_log_records LIMIT 0 FORMAT JSONEachRow",
		"SELECT " + spanColumns + " FROM tracepilot_spans LIMIT 0 FORMAT JSONEachRow",
		"SELECT project_id, environment_id, trace_id, started_at, ended_at FROM tracepilot_trace_index LIMIT 0 FORMAT JSONEachRow",
		"SELECT " + metricColumns + " FROM tracepilot_metric_points LIMIT 0 FORMAT JSONEachRow",
	} {
		if _, err := store.execute(ctx, check, nil, nil, false); err != nil {
			return err
		}
	}
	return nil
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for index, character := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if !strings.ContainsRune("0123456789abcdefABCDEF", character) {
			return false
		}
	}
	return true
}

func validateScope(projectID, environmentID string) error {
	if !validUUID(projectID) || !validUUID(environmentID) {
		return fmt.Errorf("invalid ClickHouse scope: %w", telemetry.ErrInvalidEvent)
	}
	return nil
}
