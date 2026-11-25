package retention

import "github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"

// Retention reads and writes through one shared ClickHouse client.
type Retention struct{ client *client.Client }

func New(clickhouse *client.Client) *Retention { return &Retention{client: clickhouse} }
