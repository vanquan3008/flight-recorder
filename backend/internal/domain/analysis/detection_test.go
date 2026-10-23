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
