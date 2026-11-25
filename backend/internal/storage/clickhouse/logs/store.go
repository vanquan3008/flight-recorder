package logs

import "github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"

// Logs reads and writes through one shared ClickHouse client.
type Logs struct{ client *client.Client }

func New(clickhouse *client.Client) *Logs { return &Logs{client: clickhouse} }
