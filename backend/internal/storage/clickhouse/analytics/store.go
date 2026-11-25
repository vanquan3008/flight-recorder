package analytics

import "github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"

// Analytics reads and writes through one shared ClickHouse client.
type Analytics struct{ client *client.Client }

func New(clickhouse *client.Client) *Analytics { return &Analytics{client: clickhouse} }
