export type Page<T> = { data: T[]; next_cursor?: string };

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

export type TraceDetail = { trace_id: string; spans: Span[] };

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

export type SecurityEvent = {
  id: string;
  raw_event_id: string;
  occurred_at: string;
  format: string;
  kind: string;
  action?: string;
  outcome?: string;
  severity?: string;
  source_ip?: string;
  destination_ip?: string;
  host_name?: string;
  user_name?: string;
  message?: string;
};

export type Detection = {
  id: string;
  rule_id: string;
  rule_version: number;
  security_event_id: string;
  incident_id?: string;
  group_key?: string;
  window_started_at: string;
  window_ended_at: string;
  severity: string;
  created_at: string;
};

export type SecuritySource = {
  id: string;
  name: string;
  type: string;
  format: string;
  last_seen_at: string;
  accepted_count: number;
  error_count: number;
};

export type Rule = {
  id: string;
  name: string;
  version: number;
  enabled: boolean;
  severity: string;
  field: string;
  operator: string;
  value?: string;
  threshold: number;
  group_by?: string;
  window_seconds: number;
};

export type RuleAudit = {
  id: string;
  version: number;
  action: string;
  actor_key_id?: string;
  rule: Rule;
  created_at: string;
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
