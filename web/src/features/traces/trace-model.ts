import type { Span, TopologyEdge, TopologyNode } from "../../shared/types";

export type TraceRow = {
  span: Span;
  depth: number;
  /** Span IDs of every ancestor, used to hide a collapsed subtree. */
  path: string[];
  hasChildren: boolean;
  startMs: number;
  durationMs: number;
  /** Fractions of the whole trace (0..1) for placing the bar. */
  offset: number;
  width: number;
  serviceIndex: number;
};

export type ServiceStat = { name: string; index: number; spans: number; errors: number; selfMs: number };

export type TraceModel = {
  rows: TraceRow[];
  totalMs: number;
  services: ServiceStat[];
  /** Service-level view of the request: one node per service, one edge per caller → callee pair. */
  nodes: TopologyNode[];
  edges: TopologyEdge[];
  errors: number;
  /** Spans whose parent was never received, so their subtree is attached at the root. */
  orphans: number;
};

const time = (value: string) => {
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : 0;
};

export const isError = (span: Span) => span.status_code === "ERROR";

export function buildTrace(spans: Span[]): TraceModel {
  const ids = new Set(spans.map((span) => span.span_id));
  const children = new Map<string, Span[]>();
  for (const span of spans) {
    const parent = span.parent_span_id && ids.has(span.parent_span_id) && span.parent_span_id !== span.span_id ? span.parent_span_id : "";
    children.set(parent, [...(children.get(parent) ?? []), span]);
  }
  children.forEach((group) => group.sort((left, right) => time(left.started_at) - time(right.started_at) || left.span_id.localeCompare(right.span_id)));

  const first = spans.length ? Math.min(...spans.map((span) => time(span.started_at))) : 0;
  const last = spans.length ? Math.max(...spans.map((span) => Math.max(time(span.ended_at), time(span.started_at)))) : 0;
  const totalMs = Math.max(last - first, 1);

  const serviceOrder: string[] = [];
  const serviceIndex = (name: string) => {
    let index = serviceOrder.indexOf(name);
    if (index < 0) index = serviceOrder.push(name) - 1;
    return index;
  };

  const rows: TraceRow[] = [];
  const seen = new Set<string>();
  const visit = (parent: string, depth: number, path: string[]) => {
    for (const span of children.get(parent) ?? []) {
      if (seen.has(span.span_id)) continue;
      seen.add(span.span_id);
      const startMs = time(span.started_at) - first;
      const durationMs = Math.max(time(span.ended_at) - time(span.started_at), 0);
      rows.push({
        span, depth, path, hasChildren: (children.get(span.span_id) ?? []).length > 0, startMs, durationMs,
        offset: Math.min(Math.max(startMs / totalMs, 0), 1), width: Math.min(Math.max(durationMs / totalMs, 0.004), 1),
        serviceIndex: serviceIndex(span.service_name),
      });
      visit(span.span_id, depth + 1, [...path, span.span_id]);
    }
  };
  visit("", 0, []);
  // A parent cycle would leave spans unreachable; show them rather than drop evidence.
  for (const span of spans) {
    if (seen.has(span.span_id)) continue;
    seen.add(span.span_id);
    const startMs = time(span.started_at) - first;
    const durationMs = Math.max(time(span.ended_at) - time(span.started_at), 0);
    rows.push({ span, depth: 0, path: [], hasChildren: false, startMs, durationMs, offset: Math.min(Math.max(startMs / totalMs, 0), 1), width: Math.min(Math.max(durationMs / totalMs, 0.004), 1), serviceIndex: serviceIndex(span.service_name) });
  }

  const byID = new Map(spans.map((span) => [span.span_id, span]));
  const duration = (span: Span) => Math.max(time(span.ended_at) - time(span.started_at), 0);
  const childTime = new Map<string, number>();
  const hops = new Map<string, { from: string; to: string; calls: number; errors: number; total: number; longest: number; trace: string }>();
  for (const span of spans) {
    const parent = span.parent_span_id ? byID.get(span.parent_span_id) : undefined;
    if (!parent || parent === span) continue;
    childTime.set(parent.span_id, (childTime.get(parent.span_id) ?? 0) + duration(span));
    if (parent.service_name === span.service_name) continue;
    const key = `${parent.service_name}\u0000${span.service_name}`;
    const hop = hops.get(key) ?? { from: parent.service_name, to: span.service_name, calls: 0, errors: 0, total: 0, longest: 0, trace: span.trace_id };
    hop.calls += 1;
    hop.errors += isError(span) ? 1 : 0;
    hop.total += duration(span);
    hop.longest = Math.max(hop.longest, duration(span));
    hops.set(key, hop);
  }

  const stats = new Map<string, ServiceStat>();
  for (const span of spans) {
    const stat = stats.get(span.service_name) ?? { name: span.service_name, index: serviceIndex(span.service_name), spans: 0, errors: 0, selfMs: 0 };
    stat.spans += 1;
    stat.errors += isError(span) ? 1 : 0;
    // Self time: what this span spent outside its children, so service totals do not double count nesting.
    stat.selfMs += Math.max(duration(span) - (childTime.get(span.span_id) ?? 0), 0);
    stats.set(span.service_name, stat);
  }
  const services = [...stats.values()].sort((left, right) => left.index - right.index);

  return {
    rows, totalMs, services, errors: spans.filter(isError).length, orphans: spans.filter((span) => span.parent_span_id && !ids.has(span.parent_span_id)).length,
    nodes: services.map((service) => ({ id: service.name, name: service.name, type: "service" })),
    edges: [...hops.values()].map((hop) => ({
      source_node_id: hop.from, target_node_id: hop.to, kind: "calls", call_count: hop.calls, error_count: hop.errors,
      average_latency_nanos: (hop.total / hop.calls) * 1e6, p95_latency_nanos: hop.longest * 1e6, trace_ids: [hop.trace],
    })),
  };
}

/** Rows that stay visible once the given spans are collapsed. */
export function visibleRows(rows: TraceRow[], collapsed: Set<string>): TraceRow[] {
  return collapsed.size ? rows.filter((row) => !row.path.some((ancestor) => collapsed.has(ancestor))) : rows;
}

export function axisTicks(totalMs: number, count = 4): { fraction: number; ms: number }[] {
  return Array.from({ length: count + 1 }, (_, index) => ({ fraction: index / count, ms: (totalMs * index) / count }));
}

export function formatMs(ms: number): string {
  if (ms >= 1000) return `${(ms / 1000).toFixed(ms >= 10_000 ? 1 : 2)} s`;
  if (ms >= 100) return `${Math.round(ms)} ms`;
  if (ms >= 1) return `${ms.toFixed(1)} ms`;
  return ms === 0 ? "0 ms" : `${(ms * 1000).toFixed(0)} µs`;
}
