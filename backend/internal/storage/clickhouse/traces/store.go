package traces

import "github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"

// Traces reads and writes through one shared ClickHouse client.
type Traces struct{ client *client.Client }

func New(clickhouse *client.Client) *Traces { return &Traces{client: clickhouse} }
