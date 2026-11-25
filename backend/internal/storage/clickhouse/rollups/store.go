package rollups

import "github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"

// Rollups reads and writes through one shared ClickHouse client.
type Rollups struct{ client *client.Client }

func New(clickhouse *client.Client) *Rollups { return &Rollups{client: clickhouse} }
