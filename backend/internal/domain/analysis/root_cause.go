package analysis

import (
	"math"
	"sort"
	"strings"
)

// BuildRCA ranks hypotheses and only relates measured, connected evidence.
// Scores are deterministic heuristics, not probabilities of causality.
func BuildRCA(incidentID, primaryService string, evidence []Evidence, dependencies []TopologyEdge) IncidentAnalysis {
	ordered := append([]Evidence{}, evidence...)
	sort.Slice(ordered, func(left, right int) bool {
		a, b := ordered[left], ordered[right]
		if evidencePriority(a) != evidencePriority(b) {
			return evidencePriority(a) < evidencePriority(b)
		}
		if confidence(a) != confidence(b) {
			return confidence(a) > confidence(b)
		}
		for _, pair := range [][2]string{{a.EntityID, b.EntityID}, {a.MetricName, b.MetricName}, {a.SeriesID, b.SeriesID}, {a.ID, b.ID}} {
			if pair[0] != pair[1] {
				return pair[0] < pair[1]
			}
		}
		return false
	})
	result := IncidentAnalysis{IncidentID: incidentID, Evidence: ordered, Hypotheses: make([]Hypothesis, 0), Graph: BuildSupportedEdges(ordered, dependencies)}
	for index, item := range ordered {
		if index == 3 {
			break
		}
		result.Hypotheses = append(result.Hypotheses, Hypothesis{Rank: index + 1, EntityID: item.EntityID, Statement: item.Summary, Confidence: confidence(item), EvidenceIDs: []string{item.ID}})
	}
	if len(result.Hypotheses) == 0 {
		result.Conclusion = "Insufficient evidence for root-cause analysis."
	} else {
		result.Conclusion = "Highest-ranked hypothesis for " + primaryService + ": " + result.Hypotheses[0].Statement
	}
	return result
}

// Connections show correlation, not an invented direction of causal propagation.
func BuildSupportedEdges(ordered []Evidence, dependencies []TopologyEdge) []CausalEdge {
	connected := make(map[[2]string]bool, len(dependencies)*2)
	for _, edge := range dependencies {
		connected[[2]string{edge.SourceNodeID, edge.TargetNodeID}] = true
		connected[[2]string{edge.TargetNodeID, edge.SourceNodeID}] = true
	}
	graph := make([]CausalEdge, 0)
	for index, source := range ordered {
		for _, target := range ordered[index+1:] {
			if source.ID == target.ID || source.EntityID == target.EntityID || !connected[[2]string{source.EntityID, target.EntityID}] {
				continue
			}
			if source.WindowEndedAt.Before(target.WindowStartedAt) || target.WindowEndedAt.Before(source.WindowStartedAt) || !sharedTrace(source.TraceIDs, target.TraceIDs) {
				continue
			}
			graph = append(graph, CausalEdge{SourceEntityID: evidenceNode(source), TargetEntityID: evidenceNode(target), Relationship: "correlated_dependency", Confidence: math.Min(confidence(source), confidence(target)), EvidenceIDs: []string{source.ID, target.ID}})
		}
	}
	return graph
}

func sharedTrace(left, right []string) bool {
	for _, a := range left {
		for _, b := range right {
			if a != "" && a == b {
				return true
			}
		}
	}
	return false
}

func evidencePriority(item Evidence) int {
	metric := strings.ToLower(item.MetricName)
	if item.EntityType == "external" || item.EntityType == "database" {
		return 0
	}
	if (strings.Contains(metric, "throughput") || MetricDirection(metric) == "decrease") && item.Direction == "decrease" {
		return 1
	}
	if item.EntityType == "queue" || strings.Contains(metric, "backlog") {
		return 2
	}
	if strings.Contains(metric, "latency") || strings.Contains(metric, "duration") {
		return 3
	}
	if strings.Contains(metric, "error") || item.Direction == "event" {
		return 4
	}
	return 5
}

func confidence(item Evidence) float64 {
	if item.Direction == "event" {
		return 0.7
	}
	// A zero baseline has no defined ratio; do not let its physical unit set the score.
	if item.BaselineValue == 0 {
		return 0.55
	}
	magnitude := math.Abs(item.ChangeRatio - 1)
	if math.IsNaN(magnitude) || math.IsInf(magnitude, 0) {
		return 0.55
	}
	return math.Min(0.95, 0.55+math.Min(0.4, magnitude/2))
}

func evidenceNode(item Evidence) string {
	node := item.EntityID + "#" + item.MetricName
	if item.SeriesID != "" {
		node += "#" + item.SeriesID
	}
	return node
}
