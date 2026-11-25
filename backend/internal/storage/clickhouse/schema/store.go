// Package schema applies and checks the ClickHouse migrations, and verifies that the schema the
// stores read and write is in place.
package schema

import "github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"

type Schema struct{ client *client.Client }

func New(clickhouse *client.Client) *Schema { return &Schema{client: clickhouse} }
