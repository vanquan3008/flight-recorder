package metrics

import "github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"

// Metrics reads and writes through one shared ClickHouse client.
type Metrics struct{ client *client.Client }

func New(clickhouse *client.Client) *Metrics { return &Metrics{client: clickhouse} }
