package retention

import (
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
)

func TestPartitionEndCoversEveryGranularity(t *testing.T) {
	for _, test := range []struct {
		granularity config.LogPartition
		id          string
		want        time.Time
	}{
		{config.LogPartitionDaily, "20261119", time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)},
		{config.LogPartitionWeekly, "20261116", time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)},
		{"", "20261116", time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)},
		{config.LogPartitionMonthly, "202612", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
		{config.LogPartitionYearly, "2026", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
	} {
		if got, known := partitionEnd(test.granularity, test.id); !known || !got.Equal(test.want) {
			t.Errorf("partitionEnd(%q, %q) = %s, %t; want %s", test.granularity, test.id, got, known, test.want)
		}
	}
	for _, id := range []string{"all", "2026111", "2026-11-19", "20261119'; DROP TABLE x"} {
		if _, known := partitionEnd(config.LogPartitionDaily, id); known {
			t.Errorf("partition ID %q was accepted", id)
		}
	}
}
