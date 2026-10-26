export interface Page<T> { data: T[]; next_cursor?: string }

export type PerformanceStats = {
  requests: number;
  errors: number;
  requests_per_second: number;
  error_rate: number;
  average_latency_nanos: number;
  p95_latency_nanos: number;
  p99_latency_nanos: number;
};
export type PerformanceBucket = PerformanceStats & { from: string; to: string };
export type PerformanceGroup = PerformanceStats & { service: string; endpoint?: string };
export type RequestPerformance = {
  from: string;
  to: string;
  service: string;
  endpoint: string;
  bucket_seconds: number;
  limit: number;
  summary: PerformanceStats;
  series: PerformanceBucket[];
  services: PerformanceGroup[];
  endpoints: PerformanceGroup[];
};

export type TraceSummary = {
  trace_id: string;
  started_at: string;
  ended_at: string;
  duration_nanos: number;
  root_service: string;
  span_count: number;
  error_count: number;
};

export type Span = {
  trace_id: string;
  span_id: string;
  parent_span_id?: string;
  name: string;
  started_at: string;
  ended_at: string;
  status_code: string;
  service_name: string;
  attributes: unknown;
  resource_attributes: unknown;
  events: unknown;
  links: unknown;
};

/** One page of a trace; next_cursor is present while more spans follow. */
export type TraceDetail = { trace_id: string; spans: Span[]; next_cursor?: string };

export type LogRecord = {
  event_id: string;
  occurred_at: string;
  trace_id?: string;
  span_id?: string;
  severity_text?: string;
  body: unknown;
  service_name: string;
  resource_attributes: unknown;
};

export type Incident = {
  id: string;
  primary_service: string;
  trace_id?: string;
  title: string;
  error_message?: string;
  severity: string;
  status: string;
  first_seen_at: string;
  last_seen_at: string;
  occurrence_count: number;
};

export type IncidentOccurrence = {
  id: string;
  occurred_at: string;
  service_name: string;
  trace_id?: string;
  event_type: string;
  operation?: string;
  error_type?: string;
  error_message?: string;
};

export type IncidentEvent = Omit<IncidentOccurrence, "id">;

export type StatusChange = {
  id: string;
  previous_status: string;
  status: string;
  actor_key_id?: string;
  changed_at: string;
};

export type TopologyNode = { id: string; type: string; name: string };
export type TopologyEdge = {
  source_node_id: string;
  target_node_id: string;
  kind: string;
  call_count: number;
  error_count: number;
  average_latency_nanos: number;
  p95_latency_nanos: number;
  trace_ids: string[];
};
export type Topology = { from: string; to: string; nodes: TopologyNode[]; edges: TopologyEdge[] };
export type TelemetryQuality = { from: string; to: string; spans: number; orphan_spans: number; logs: number; uncorrelated_logs: number; metric_points: number };
export type IncidentEvidence = {
	series_id?: string;
  id: string;
  signal_type: string;
  entity_type: string;
  entity_id: string;
  metric_name: string;
  direction: string;
  observed_value: number;
  baseline_value: number;
  change_ratio: number;
  trace_ids: string[];
  summary: string;
};
export type IncidentAnalysis = {
  incident_id: string;
  conclusion: string;
  hypotheses: { rank: number; entity_id: string; statement: string; confidence: number; evidence_ids: string[] }[];
  evidence: IncidentEvidence[];
  causal_graph: { source_entity_id: string; target_entity_id: string; relationship: string; confidence: number; evidence_ids: string[] }[];
};
