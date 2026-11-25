package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

// Insert sends rows to table as one JSONEachRow insert and returns once ClickHouse has stored the
// whole batch. An empty batch sends nothing.
func Insert[Row any](ctx context.Context, clickhouse *Client, table string, rows []Row) error {
	if len(rows) == 0 {
		return nil
	}
	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	for _, row := range rows {
		if err := encoder.Encode(row); err != nil {
			return fmt.Errorf("encode ClickHouse row: %w", telemetry.ErrInvalidEvent)
		}
	}
	_, err := clickhouse.Execute(ctx, "INSERT INTO "+table+" FORMAT JSONEachRow", nil, payload.Bytes(), true)
	return err
}
