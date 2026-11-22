// Package analysis contains the read models for topology, metrics, incidents,
// and evidence-backed root-cause hypotheses.
package analysis

import "time"

type Filter struct {
	EnvironmentID string
	Service       string
	Metric        string
	From          time.Time
	To            time.Time
	Since         time.Time // Retention cutoff for the window and any parent spans it reads.
	Limit         int
}

type TopologyNode struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
}

type TopologyEdge struct {
	SourceNodeID       string   `json:"source_node_id"`
	TargetNodeID       string   `json:"target_node_id"`
	Kind               string   `json:"kind"`
	CallCount          int64    `json:"call_count"`
	ErrorCount         int64    `json:"error_count"`
	AverageLatencyNano int64    `json:"average_latency_nanos"`
	P95LatencyNano     int64    `json:"p95_latency_nanos"`
	TraceIDs           []string `json:"trace_ids"`
}

type Topology struct {
	From  time.Time      `json:"from"`
	To    time.Time      `json:"to"`
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}

type MetricPoint struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Unit       string         `json:"unit"`
	Type       string         `json:"type"`
	Service    string         `json:"service"`
	OccurredAt time.Time      `json:"occurred_at"`
	Value      *float64       `json:"value,omitempty"`
	Count      *int64         `json:"count,omitempty"`
	Sum        *float64       `json:"sum,omitempty"`
	Attributes map[string]any `json:"attributes"`
	Exemplars  []Exemplar     `json:"exemplars"`
}

type Exemplar struct {
	OccurredAt time.Time      `json:"occurred_at"`
	Value      float64        `json:"value"`
	TraceID    string         `json:"trace_id,omitempty"`
	SpanID     string         `json:"span_id,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

type Quality struct {
	From             time.Time `json:"from"`
	To               time.Time `json:"to"`
	Spans            int64     `json:"spans"`
	OrphanSpans      int64     `json:"orphan_spans"`
	Logs             int64     `json:"logs"`
	UncorrelatedLogs int64     `json:"uncorrelated_logs"`
	MetricPoints     int64     `json:"metric_points"`
}

type Evidence struct {
	ID              string    `json:"id"`
	SeriesID        string    `json:"series_id,omitempty"`
	SignalType      string    `json:"signal_type"`
	EntityType      string    `json:"entity_type"`
	EntityID        string    `json:"entity_id"`
	MetricName      string    `json:"metric_name"`
	Direction       string    `json:"direction"`
	ObservedValue   float64   `json:"observed_value"`
	BaselineValue   float64   `json:"baseline_value"`
	ChangeRatio     float64   `json:"change_ratio"`
	WindowStartedAt time.Time `json:"window_started_at"`
	WindowEndedAt   time.Time `json:"window_ended_at"`
	TraceIDs        []string  `json:"trace_ids"`
	Summary         string    `json:"summary"`
}

type Hypothesis struct {
	Rank        int      `json:"rank"`
	EntityID    string   `json:"entity_id"`
	Statement   string   `json:"statement"`
	Confidence  float64  `json:"confidence"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type CausalEdge struct {
	SourceEntityID string   `json:"source_entity_id"`
	TargetEntityID string   `json:"target_entity_id"`
	Relationship   string   `json:"relationship"`
	Confidence     float64  `json:"confidence"`
	EvidenceIDs    []string `json:"evidence_ids"`
}

type IncidentAnalysis struct {
	IncidentID string       `json:"incident_id"`
	Conclusion string       `json:"conclusion"`
	Hypotheses []Hypothesis `json:"hypotheses"`
	Evidence   []Evidence   `json:"evidence"`
	Graph      []CausalEdge `json:"causal_graph"`
}
