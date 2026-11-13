package signals

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func TestHistogramBucketEncodingKeepsUnboundedCount(t *testing.T) {
	encoded, err := encodeHistogramBuckets([]telemetry.HistogramBucket{{UpperBound: 1, Count: 2}, {UpperBound: math.Inf(1), Count: 3}})
	if err != nil {
		t.Fatal(err)
	}
	var buckets []struct {
		Bound *float64 `json:"bound"`
		Count uint64   `json:"count"`
	}
	if err := json.Unmarshal(encoded, &buckets); err != nil {
		t.Fatal(err)
	}
	if len(buckets) != 2 || buckets[0].Bound == nil || *buckets[0].Bound != 1 || buckets[1].Bound != nil || buckets[1].Count != 3 {
		t.Fatalf("lost bucket: %s", encoded)
	}
	for _, bound := range []float64{math.NaN(), math.Inf(-1)} {
		if _, err := encodeHistogramBuckets([]telemetry.HistogramBucket{{UpperBound: bound}}); err == nil {
			t.Fatal("invalid bound accepted")
		}
	}
}
