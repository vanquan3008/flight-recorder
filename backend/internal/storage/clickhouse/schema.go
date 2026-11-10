package clickhouse

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

//go:embed schema.sql
var schemaSQL string

type schemaState struct {
	Engine    string `json:"engine"`
	Partition string `json:"partition_key"`
	Sorting   string `json:"sorting_key"`
	DDL       string `json:"create_table_query"`
}

func (store *Store) schema(ctx context.Context) (schemaState, error) {
	data, err := store.execute(ctx, `SELECT engine, partition_key, sorting_key, create_table_query
FROM system.tables WHERE database = {database:String} AND name = 'tracepilot_logs' FORMAT JSONEachRow`,
		map[string]string{"database": store.cfg.Database}, nil, false)
	if err != nil {
		return schemaState{}, err
	}
	var state schemaState
	if err := json.Unmarshal(data, &state); err != nil {
		return state, errors.New("ClickHouse log schema is missing; run migrate clickhouse up")
	}
	return state, nil
}

var validPartitionExprs = []string{
	"toYYYYMMDD(occurred_at,'UTC')",
	"toYYYYMM(occurred_at,'UTC')",
	"toYear(occurred_at,'UTC')",
	"toStartOfWeek(occurred_at,1,'UTC')",
}

func (store *Store) Ready(ctx context.Context) error {
	state, err := store.schema(ctx)
	if err != nil {
		return err
	}
	compact := func(value string) string { return strings.Join(strings.Fields(value), "") }
	got := compact(state.Partition)
	partitionOK := false
	for _, expr := range validPartitionExprs {
		if got == expr {
			partitionOK = true
			break
		}
	}
	if !strings.HasSuffix(state.Engine, "ReplacingMergeTree") || !partitionOK ||
		compact(state.Sorting) != "project_id,environment_id,service_name,toStartOfMillisecond(occurred_at),event_id" {
		return errors.New("ClickHouse log schema must use a supported time-based partition (daily/monthly/yearly/weekly) and the expected deduplication key")
	}
	ttl := store.ttl()
	if (ttl == "" && strings.Contains(state.DDL, "TTL ")) || (ttl != "" && !strings.Contains(state.DDL, ttl)) {
		return errors.New("ClickHouse log TTL differs from retention configuration; run migrate clickhouse up")
	}
	_, err = store.execute(ctx, "SELECT "+logColumns+" FROM tracepilot_logs LIMIT 0 FORMAT JSONEachRow", nil, nil, false)
	return err
}

// Migrate is explicit: serving traffic never creates or changes ClickHouse tables.
func (store *Store) Migrate(ctx context.Context) error {
	if _, err := store.execute(ctx, "CREATE DATABASE IF NOT EXISTS `"+store.cfg.Database+"`", nil, nil, false); err != nil {
		return err
	}
	ddl := strings.Replace(schemaSQL, "__PARTITION__", store.cfg.LogPartition.PartitionExpr(), 1)
	ddl = strings.Replace(ddl, "__TTL__", store.ttl(), 1)
	if _, err := store.execute(ctx, ddl, nil, nil, false); err != nil {
		return err
	}
	state, err := store.schema(ctx)
	if err != nil {
		return err
	}
	ttl := store.ttl()
	if ttl != "" {
		_, err = store.execute(ctx, "ALTER TABLE tracepilot_logs MODIFY "+ttl, nil, nil, false)
	} else if strings.Contains(state.DDL, "TTL ") {
		_, err = store.execute(ctx, "ALTER TABLE tracepilot_logs REMOVE TTL", nil, nil, false)
	}
	if err != nil {
		return err
	}
	return store.Ready(ctx)
}

func (store *Store) ttl() string {
	if store.maxAge <= 0 {
		return ""
	}
	return fmt.Sprintf("TTL occurred_at + toIntervalSecond(%d)", max(1, int64(store.maxAge.Seconds())))
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
