package signals

import (
	"encoding/json"
	"errors"
	"math"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

// JSON null is the unbounded upper edge; finite bounds remain JSON numbers.
func encodeHistogramBuckets(buckets []telemetry.HistogramBucket) ([]byte, error) {
	if len(buckets) == 0 {
		return nil, nil
	}
	type bucket struct {
		Bound *float64 `json:"bound"`
		Count uint64   `json:"count"`
	}
	encoded := make([]bucket, len(buckets))
	for index, item := range buckets {
		if math.IsNaN(item.UpperBound) || math.IsInf(item.UpperBound, -1) {
			return nil, errors.New("histogram bound is invalid")
		}
		encoded[index].Count = item.Count
		if !math.IsInf(item.UpperBound, 1) {
			bound := item.UpperBound
			encoded[index].Bound = &bound
		}
	}
	return json.Marshal(encoded)
}
