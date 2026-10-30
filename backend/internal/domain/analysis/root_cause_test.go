package analysis

import (
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestRCARequiresConnectedContemporaneousEvidenceWithSharedTrace(t *testing.T) {
	now := time.Now().UTC()
	dependency := Evidence{ID: "external", EntityType: "external", EntityID: "external:payments", MetricName: "latency", Direction: "increase", BaselineValue: 1, ChangeRatio: 2, WindowStartedAt: now.Add(-time.Minute), WindowEndedAt: now, TraceIDs: []string{"shared"}}
	symptom := Evidence{ID: "error", EntityType: "service", EntityID: "service:api", MetricName: "error_rate", Direction: "increase", BaselineValue: 1, ChangeRatio: 3, WindowStartedAt: now.Add(-time.Minute), WindowEndedAt: now, TraceIDs: []string{"shared"}}
	edges := []TopologyEdge{{SourceNodeID: "service:api", TargetNodeID: "external:payments"}}
	for _, test := range []struct {
		name         string
		change       func(*Evidence)
		dependencies []TopologyEdge
		want         int
	}{
		{"supported", func(*Evidence) {}, edges, 1},
		{"unconnected", func(*Evidence) {}, nil, 0},
		{"different trace", func(e *Evidence) { e.TraceIDs = []string{"other"} }, edges, 0},
		{"empty trace", func(e *Evidence) { e.TraceIDs = []string{""} }, edges, 0},
		{"stale window", func(e *Evidence) { e.WindowStartedAt = now.Add(-time.Hour); e.WindowEndedAt = now.Add(-time.Hour) }, edges, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := symptom
			test.change(&item)
			result := BuildRCA("incident", "api", []Evidence{item, dependency}, test.dependencies)
			if len(result.Graph) != test.want || result.Hypotheses[0].EvidenceIDs[0] != "external" {
				t.Fatalf("unexpected analysis: %#v", result)
			}
			if test.want > 0 && result.Graph[0].Relationship != "correlated_dependency" {
				t.Fatal("correlation presented as causality")
			}
		})
	}
}

func TestRCARankingIsStableAcrossInputOrderAndZeroBaselineUnits(t *testing.T) {
	evidence := []Evidence{
		{ID: "b", EntityID: "service:b", MetricName: "latency", Direction: "increase", ObservedValue: 1000000000},
		{ID: "a", EntityID: "service:a", MetricName: "latency", Direction: "increase", ObservedValue: 1},
	}
	want := BuildRCA("incident", "api", evidence, nil)
	slices.Reverse(evidence)
	got := BuildRCA("incident", "api", evidence, nil)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("ranking changed on permutation: %#v / %#v", want, got)
	}
	if got.Hypotheses[0].Confidence != got.Hypotheses[1].Confidence {
		t.Fatal("zero-baseline score depends on unit")
	}
}

func TestRCARankNeverPlacesWeakerEvidenceAboveStronger(t *testing.T) {
	evidence := []Evidence{
		// An external dependency with no measured baseline scores 0.55.
		{ID: "weak-external", EntityID: "external:payments", EntityType: "external", MetricName: "latency", Direction: "increase", BaselineValue: 0, ObservedValue: 2},
		// A tenfold error-rate jump on the service itself scores far higher.
		{ID: "strong-error", EntityID: "service:api", EntityType: "service", MetricName: "error_rate", Direction: "increase", BaselineValue: 0.01, ObservedValue: 0.5, ChangeRatio: 50},
	}
	result := BuildRCA("incident", "api", evidence, nil)
	if len(result.Hypotheses) != 2 {
		t.Fatalf("hypotheses = %d", len(result.Hypotheses))
	}
	if result.Hypotheses[0].EvidenceIDs[0] != "strong-error" {
		t.Fatalf("top hypothesis is %v, want the strongest evidence", result.Hypotheses[0].EvidenceIDs)
	}
	for index := 1; index < len(result.Hypotheses); index++ {
		if result.Hypotheses[index].Confidence > result.Hypotheses[index-1].Confidence {
			t.Fatalf("rank %d has higher confidence than rank %d", index+1, index)
		}
	}
}
