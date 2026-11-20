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

/** Epoch milliseconds keeping the sub-millisecond digits Date.parse drops; 0 when unparsable. */
export function timeMs(value: string): number {
  const match = /^(.*\.\d{3})(\d+)(.*)$/.exec(value);
  const parsed = Date.parse(match ? match[1] + match[3] : value);
  if (!Number.isFinite(parsed)) return 0;
  return match ? parsed + Number(`0.${match[2]}`) : parsed;
}

function spanDurationMs(span: Span): number {
  if (typeof span.duration_nanos === "number" && Number.isFinite(span.duration_nanos)) return Math.max(span.duration_nanos / 1e6, 0);
  return Math.max(timeMs(span.ended_at) - timeMs(span.started_at), 0);
}

export const isError = (span: Span) => span.status_code === "ERROR";

/** Linear-interpolated percentile, as PostgreSQL percentile_cont computes it. */
export function percentile(values: number[], fraction: number): number {
  if (!values.length) return 0;
  const sorted = [...values].sort((left, right) => left - right);
  const rank = fraction * (sorted.length - 1);
  const low = Math.floor(rank), high = Math.ceil(rank);
  return sorted[low] + (sorted[high] - sorted[low]) * (rank - low);
}

/** Time a span spent outside the union of its children, clipped to the span. */
export function selfTimeMs(start: number, duration: number, children: { start: number; duration: number }[]): number {
  const end = start + duration;
  const intervals = children.map((child) => [Math.max(child.start, start), Math.min(child.start + child.duration, end)])
    .filter(([from, to]) => to > from).sort((left, right) => left[0] - right[0]);
  let covered = 0, coveredUntil = start;
  for (const [from, to] of intervals) {
    const gapStart = Math.max(from, coveredUntil);
    if (to > gapStart) {
      covered += to - gapStart;
      coveredUntil = to;
    }
  }
  return Math.max(duration - covered, 0);
}

export function buildTrace(spans: Span[]): TraceModel {
  const ids = new Set(spans.map((span) => span.span_id));
  const startOf = new Map(spans.map((span) => [span, timeMs(span.started_at)]));
  const durationOf = new Map(spans.map((span) => [span, spanDurationMs(span)]));
  const start = (span: Span) => startOf.get(span) ?? 0;
  const duration = (span: Span) => durationOf.get(span) ?? 0;
  const children = new Map<string, Span[]>();
  for (const span of spans) {
    const parent = span.parent_span_id && ids.has(span.parent_span_id) && span.parent_span_id !== span.span_id ? span.parent_span_id : "";
    children.set(parent, [...(children.get(parent) ?? []), span]);
  }
  children.forEach((group) => group.sort((left, right) => start(left) - start(right) || left.span_id.localeCompare(right.span_id)));

  const first = spans.length ? Math.min(...spans.map(start)) : 0;
  const last = spans.length ? Math.max(...spans.map((span) => start(span) + duration(span))) : 0;
  const totalMs = Math.max(last - first, 1);

  const serviceOrder: string[] = [];
  const serviceIndex = (name: string) => {
    let index = serviceOrder.indexOf(name);
    if (index < 0) index = serviceOrder.push(name) - 1;
    return index;
  };

  const rows: TraceRow[] = [];
  const seen = new Set<string>();
  const row = (span: Span, depth: number, path: string[], hasChildren: boolean): TraceRow => {
    const startMs = start(span) - first;
    const durationMs = duration(span);
    return {
      span, depth, path, hasChildren, startMs, durationMs,
      offset: Math.min(Math.max(startMs / totalMs, 0), 1), width: Math.min(Math.max(durationMs / totalMs, 0.004), 1),
      serviceIndex: serviceIndex(span.service_name),
    };
  };
  const visit = (parent: string, depth: number, path: string[]) => {
    for (const span of children.get(parent) ?? []) {
      if (seen.has(span.span_id)) continue;
      seen.add(span.span_id);
      rows.push(row(span, depth, path, (children.get(span.span_id) ?? []).length > 0));
      visit(span.span_id, depth + 1, [...path, span.span_id]);
    }
  };
  visit("", 0, []);
  // A parent cycle would leave spans unreachable; show them rather than drop evidence.
  for (const span of spans) {
    if (seen.has(span.span_id)) continue;
    seen.add(span.span_id);
    rows.push(row(span, 0, [], false));
  }

  const byID = new Map(spans.map((span) => [span.span_id, span]));
  const childSpans = new Map<string, Span[]>();
  const hops = new Map<string, { from: string; to: string; errors: number; durations: number[]; trace: string }>();
  for (const span of spans) {
    const parent = span.parent_span_id ? byID.get(span.parent_span_id) : undefined;
    if (!parent || parent === span) continue;
    childSpans.set(parent.span_id, [...(childSpans.get(parent.span_id) ?? []), span]);
    if (parent.service_name === span.service_name) continue;
    const key = `${parent.service_name}\u0000${span.service_name}`;
    const hop = hops.get(key) ?? { from: parent.service_name, to: span.service_name, errors: 0, durations: [], trace: span.trace_id };
    hop.errors += isError(span) ? 1 : 0;
    hop.durations.push(duration(span));
    hops.set(key, hop);
  }

  const stats = new Map<string, ServiceStat>();
  for (const span of spans) {
    const stat = stats.get(span.service_name) ?? { name: span.service_name, index: serviceIndex(span.service_name), spans: 0, errors: 0, selfMs: 0 };
    stat.spans += 1;
    stat.errors += isError(span) ? 1 : 0;
    const kids = (childSpans.get(span.span_id) ?? []).map((child) => ({ start: start(child), duration: duration(child) }));
    stat.selfMs += selfTimeMs(start(span), duration(span), kids);
    stats.set(span.service_name, stat);
  }
  const services = [...stats.values()].sort((left, right) => left.index - right.index);

  return {
    rows, totalMs, services, errors: spans.filter(isError).length, orphans: spans.filter((span) => span.parent_span_id && !ids.has(span.parent_span_id)).length,
    nodes: services.map((service) => ({ id: service.name, name: service.name, type: "service" })),
    edges: [...hops.values()].map((hop) => ({
      source_node_id: hop.from, target_node_id: hop.to, kind: "calls", call_count: hop.durations.length, error_count: hop.errors,
      average_latency_nanos: (hop.durations.reduce((sum, value) => sum + value, 0) / hop.durations.length) * 1e6,
      p95_latency_nanos: percentile(hop.durations, 0.95) * 1e6, trace_ids: [hop.trace],
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
