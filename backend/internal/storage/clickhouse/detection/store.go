package detection

import "github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"

// Detector reads and writes through one shared ClickHouse client.
type Detector struct{ client *client.Client }

func New(clickhouse *client.Client) *Detector { return &Detector{client: clickhouse} }
