package analysis

import (
	"math"
	"strings"
	"time"
)

type WindowStats struct {
	Calls        int64
	Errors       int64
	LatencyNanos float64
}

type Signal struct {
	Metric    string
	Direction string
	Observed  float64
	Baseline  float64
}

// Sample-size floors. Latency is a mean, so a handful of calls lets one slow
// request open an incident; throughput needs a baseline busy enough that
// halving it means something. Both were lower and produced noise at low traffic.
const (
	minLatencyCalls            = 10
	minThroughputBaselineCalls = 30
)

// EvaluateService separates policy from SQL and requires measured baselines.
func EvaluateService(current, baseline WindowStats, currentDuration, baselineDuration time.Duration) []Signal {
	signals := make([]Signal, 0, 3)
	if current.Calls >= 20 && baseline.Calls >= 20 && current.Errors >= 3 {
		observed := float64(current.Errors) / float64(current.Calls)
		reference := float64(baseline.Errors) / float64(baseline.Calls)
		if observed > reference*1.5 && observed-reference >= 0.01 {
			signals = append(signals, Signal{"error_rate", "increase", observed, reference})
		}
	}
	if current.Calls >= minLatencyCalls && baseline.Calls >= minLatencyCalls && current.LatencyNanos >= baseline.LatencyNanos*1.5 && current.LatencyNanos-baseline.LatencyNanos >= float64(50*time.Millisecond) {
		signals = append(signals, Signal{"latency", "increase", current.LatencyNanos, baseline.LatencyNanos})
	}
	if baseline.Calls >= minThroughputBaselineCalls && currentDuration > 0 && baselineDuration > 0 {
		observed := float64(current.Calls) / currentDuration.Seconds()
		reference := float64(baseline.Calls) / baselineDuration.Seconds()
		if observed < reference*0.5 {
			signals = append(signals, Signal{"throughput", "decrease", observed, reference})
		}
	}
	return signals
}

// MetricDirection uses complete supported signal tokens, not a "queue" guess.
func MetricDirection(name string) string {
	tokens := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return r == '.' || r == '_' || r == '/' })
	for _, token := range tokens {
		switch token {
		case "throughput", "processed", "completed":
			return "decrease"
		}
	}
	for _, token := range tokens {
		switch token {
		case "backlog", "depth", "lag", "latency", "duration", "error", "errors", "failures":
			return "increase"
		}
	}
	return ""
}

func IsDeviation(direction string, observed, baseline float64) bool {
	if math.IsNaN(observed) || math.IsNaN(baseline) || math.IsInf(observed, 0) || math.IsInf(baseline, 0) || observed < 0 || baseline < 0 || math.Abs(observed-baseline) <= 1e-12 {
		return false
	}
	return (direction == "increase" && observed >= baseline*1.5) || (direction == "decrease" && observed <= baseline*0.5)
}

// Latency evidence uses seconds. Unknown units do not become comparable values.
func MetricScale(name, unit string) (float64, bool) {
	name = strings.ToLower(name)
	if !strings.Contains(name, "latency") && !strings.Contains(name, "duration") {
		return 1, true
	}
	switch unit {
	case "s":
		return 1, true
	case "ms":
		return 1e-3, true
	case "us", "µs":
		return 1e-6, true
	case "ns":
		return 1e-9, true
	default:
		return 0, false
	}
}
