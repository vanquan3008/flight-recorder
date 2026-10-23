import { describe, expect, it } from "vitest";
import type { Span } from "../../shared/types";
import { axisTicks, buildTrace, formatMs, visibleRows } from "./trace-model";

const base = Date.parse("2026-10-01T00:00:00Z");
const span = (id: string, service: string, startMs: number, endMs: number, extra: Partial<Span> = {}): Span => ({
  trace_id: "t1", span_id: id, name: `${service}:${id}`, started_at: new Date(base + startMs).toISOString(), ended_at: new Date(base + endMs).toISOString(),
  status_code: "OK", service_name: service, attributes: {}, resource_attributes: {}, events: [], links: [], ...extra,
});

// gateway 0-250 → order 20-230 → db 40-215, order → cache 10-20 is sibling work inside order.
const request = [
  span("gw", "gateway", 0, 250),
  span("order", "order", 20, 230, { parent_span_id: "gw" }),
  span("db", "database", 40, 215, { parent_span_id: "order" }),
  span("cache", "cache", 220, 228, { parent_span_id: "order", status_code: "ERROR" }),
];

describe("buildTrace", () => {
  it("orders spans as a tree with depth, offsets and widths relative to the whole trace", () => {
    const model = buildTrace(request);
    expect(model.totalMs).toBe(250);
    expect(model.rows.map((row) => [row.span.span_id, row.depth])).toEqual([["gw", 0], ["order", 1], ["db", 2], ["cache", 2]]);
    const db = model.rows[2];
    expect(db.offset).toBeCloseTo(40 / 250);
    expect(db.width).toBeCloseTo(175 / 250);
    expect(model.rows[1].hasChildren).toBe(true);
    expect(model.rows[2].hasChildren).toBe(false);
  });

  it("derives the service flow: one edge per caller → callee pair with calls, errors and latency", () => {
    const model = buildTrace([...request, span("db2", "database", 100, 120, { parent_span_id: "order" })]);
    expect(model.nodes.map((node) => node.name)).toEqual(["gateway", "order", "database", "cache"]);
    const orderToDb = model.edges.find((edge) => edge.source_node_id === "order" && edge.target_node_id === "database")!;
    expect(orderToDb.call_count).toBe(2);
    expect(orderToDb.average_latency_nanos).toBeCloseTo(((175 + 20) / 2) * 1e6);
    expect(model.edges.find((edge) => edge.target_node_id === "cache")!.error_count).toBe(1);
    expect(model.errors).toBe(1);
  });

  it("does not turn calls within one service into flow edges", () => {
    const model = buildTrace([span("a", "api", 0, 100), span("b", "api", 10, 50, { parent_span_id: "a" })]);
    expect(model.edges).toEqual([]);
    expect(model.nodes).toHaveLength(1);
  });

  it("attributes self time so nested services are not double counted", () => {
    const model = buildTrace(request);
    const self = Object.fromEntries(model.services.map((service) => [service.name, service.selfMs]));
    expect(self.gateway).toBe(40);
    expect(self.order).toBe(210 - 175 - 8);
    expect(self.database).toBe(175);
    expect(model.services.reduce((sum, service) => sum + service.selfMs, 0)).toBeLessThanOrEqual(250);
  });

  it("keeps spans whose parent was never received and reports them", () => {
    const model = buildTrace([span("a", "api", 0, 100), span("b", "worker", 10, 50, { parent_span_id: "lost" })]);
    expect(model.orphans).toBe(1);
    expect(model.rows.map((row) => [row.span.span_id, row.depth])).toEqual([["a", 0], ["b", 0]]);
  });

  it("survives parent cycles, self parents, zero-length and unparsable spans", () => {
    const model = buildTrace([
      span("a", "x", 0, 10, { parent_span_id: "b" }), span("b", "x", 0, 10, { parent_span_id: "a" }),
      span("c", "x", 5, 5, { parent_span_id: "c" }), span("d", "x", 0, 0, { started_at: "not a date", ended_at: "" }),
    ]);
    expect(model.rows).toHaveLength(4);
    expect(model.rows.every((row) => Number.isFinite(row.offset) && row.width > 0)).toBe(true);
  });

  it("handles an empty trace", () => {
    expect(buildTrace([])).toMatchObject({ rows: [], totalMs: 1, edges: [], nodes: [] });
  });
});

describe("visibleRows", () => {
  it("hides the whole subtree of a collapsed span but keeps the span itself", () => {
    const model = buildTrace(request);
    expect(visibleRows(model.rows, new Set(["order"])).map((row) => row.span.span_id)).toEqual(["gw", "order"]);
    expect(visibleRows(model.rows, new Set()).length).toBe(4);
  });
});

describe("axis helpers", () => {
  it("places ticks evenly from 0 to the trace total", () => {
    expect(axisTicks(250).map((tick) => tick.ms)).toEqual([0, 62.5, 125, 187.5, 250]);
  });
  it("formats durations at readable scales", () => {
    expect([formatMs(0), formatMs(0.25), formatMs(12.34), formatMs(250), formatMs(1500), formatMs(12_000)]).toEqual(["0 ms", "250 µs", "12.3 ms", "250 ms", "1.50 s", "12.0 s"]);
  });
});
