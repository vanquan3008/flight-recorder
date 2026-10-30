package analysis

import (
	"math"
	"testing"
	"time"
)

func TestServiceDetectionRejectsImprovingErrorsAndDetectsSilence(t *testing.T) {
	for _, test := range []struct {
		name                      string
		current, baseline         WindowStats
		wantError, wantThroughput bool
	}{
		{"improving", WindowStats{Calls: 100, Errors: 3}, WindowStats{Calls: 600, Errors: 60}, false, false},
		{"low volume", WindowStats{Calls: 2, Errors: 2}, WindowStats{Calls: 600}, false, true},
		{"new errors", WindowStats{Calls: 100, Errors: 3}, WindowStats{Calls: 600}, true, false},
		{"silent", WindowStats{}, WindowStats{Calls: 600}, false, true},
		{"no baseline", WindowStats{}, WindowStats{}, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			signals := EvaluateService(test.current, test.baseline, 5*time.Minute, 30*time.Minute)
			hasError, hasThroughput := false, false
			for _, signal := range signals {
				hasError = hasError || signal.Metric == "error_rate"
				hasThroughput = hasThroughput || signal.Metric == "throughput"
			}
			if hasError != test.wantError || hasThroughput != test.wantThroughput {
				t.Fatalf("signals = %#v", signals)
			}
		})
	}
}

func TestMetricPolicyRecognizesCompleteTokensAndUnits(t *testing.T) {
	for name, want := range map[string]string{"queue.processed": "decrease", "queue.backlog": "increase", "queue.size": "", "terror": "", "HTTP.DURATION": "increase"} {
		if got := MetricDirection(name); got != want {
			t.Errorf("%s: %s want %s", name, got, want)
		}
	}
	if scale, supported := MetricScale("HTTP.DURATION", "ms"); !supported || scale != .001 {
		t.Fatal("duration unit not normalized")
	}
	if _, supported := MetricScale("http.duration", "unknown"); supported {
		t.Fatal("unknown latency unit supported")
	}
	if IsDeviation("increase", math.NaN(), 1) || IsDeviation("decrease", 0, 0) || IsDeviation("increase", -1, 1) {
		t.Fatal("invalid/unchanged sample produced anomaly")
	}
	if !IsDeviation("decrease", 0, 10) {
		t.Fatal("zero throughput was excluded")
	}
}

func TestServiceDetectionNeedsEnoughCallsForLatencyAndThroughput(t *testing.T) {
	slow := func(calls int64) (WindowStats, WindowStats) {
		return WindowStats{Calls: calls, LatencyNanos: float64(400 * time.Millisecond)}, WindowStats{Calls: calls, LatencyNanos: float64(100 * time.Millisecond)}
	}
	has := func(signals []Signal, metric string) bool {
		for _, signal := range signals {
			if signal.Metric == metric {
				return true
			}
		}
		return false
	}
	current, baseline := slow(minLatencyCalls - 1)
	if has(EvaluateService(current, baseline, 5*time.Minute, 30*time.Minute), "latency") {
		t.Fatal("a handful of calls opened a latency incident")
	}
	current, baseline = slow(minLatencyCalls)
	if !has(EvaluateService(current, baseline, 5*time.Minute, 30*time.Minute), "latency") {
		t.Fatal("a real latency regression with enough calls was missed")
	}
	quiet := WindowStats{Calls: minThroughputBaselineCalls - 1}
	if has(EvaluateService(WindowStats{}, quiet, 5*time.Minute, 30*time.Minute), "throughput") {
		t.Fatal("a nearly idle service was reported as having lost traffic")
	}
	busy := WindowStats{Calls: minThroughputBaselineCalls}
	if !has(EvaluateService(WindowStats{}, busy, 5*time.Minute, 30*time.Minute), "throughput") {
		t.Fatal("silence on a busy service was missed")
	}
}
