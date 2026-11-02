import { describe, expect, it } from "vitest";
import type { TopologyEdge, TopologyNode } from "../types";
import { columnGap, edgeLabelPoint, edgePath, layoutTopology, nodeHeight, nodeWidth, truncate } from "./graph-layout";

const node = (id: string, type = "service"): TopologyNode => ({ id, type, name: id });
const edge = (source: string, target: string): TopologyEdge => ({ source_node_id: source, target_node_id: target, kind: "calls", call_count: 1, error_count: 0, average_latency_nanos: 1, p95_latency_nanos: 1, trace_ids: [] });

describe("layoutTopology", () => {
  it("places callers left of callees", () => {
    const layout = layoutTopology([node("web"), node("api"), node("db", "database")], [edge("web", "api"), edge("api", "db")]);
    const x = (id: string) => layout.byID.get(id)!.x;
    expect(x("web")).toBeLessThan(x("api"));
    expect(x("api")).toBeLessThan(x("db"));
  });

  it("moves services with no observed calls into their own band below the graph", () => {
    const layout = layoutTopology([node("web"), node("api"), node("lonely")], [edge("web", "api")]);
    const lonely = layout.byID.get("lonely")!;
    expect(lonely.isolated).toBe(true);
    expect(lonely.y).toBeGreaterThan(layout.byID.get("api")!.y);
    expect(layout.height).toBeGreaterThan(lonely.y);
  });

  it("renders a graph made only of isolated nodes and an empty graph", () => {
    expect(layoutTopology([node("a"), node("b")], []).nodes.every((item) => item.isolated)).toBe(true);
    expect(layoutTopology([], []).nodes).toEqual([]);
  });

  it("tolerates cycles, self calls and edges that reference unknown nodes", () => {
    const layout = layoutTopology([node("a"), node("b"), node("c")], [edge("a", "b"), edge("b", "c"), edge("c", "a"), edge("a", "a"), edge("a", "ghost")]);
    expect(layout.nodes).toHaveLength(3);
    expect(new Set(layout.nodes.map((item) => `${item.x}:${item.y}`)).size).toBe(3);
  });

  it("keeps long names readable by truncating the visible label only", () => {
    expect(truncate("payments-reconciliation-worker-eu-west")).toHaveLength(22);
    expect(truncate("short")).toBe("short");
  });

  it("lays out a representative large graph quickly with no overlapping nodes", () => {
    const nodes = Array.from({ length: 400 }, (_, index) => node(`svc-${index}`));
    const edges = nodes.slice(1).map((item, index) => edge(`svc-${Math.floor(index / 3)}`, item.id));
    const started = performance.now();
    const layout = layoutTopology(nodes, edges);
    expect(performance.now() - started).toBeLessThan(500);
    expect(new Set(layout.nodes.map((item) => `${item.x}:${item.y}`)).size).toBe(400);
  });
});

describe("edgeLabelPoint", () => {
  it("sits in the middle of the gap between neighbouring columns", () => {
    const layout = layoutTopology([node("web"), node("api")], [edge("web", "api")]);
    const from = layout.byID.get("web")!;
    const to = layout.byID.get("api")!;
    const label = edgeLabelPoint(from, to);
    expect(label.x).toBeCloseTo(from.x + nodeWidth + columnGap / 2, 0);
    expect(label.y).toBeCloseTo(from.y + nodeHeight / 2, 0);
    expect(edgePath(from, to)).toContain("C");
  });

  it("keeps the label in the gap after the caller when the call skips a column", () => {
    const layout = layoutTopology([node("a"), node("b"), node("c")], [edge("a", "b"), edge("b", "c"), edge("a", "c")]);
    const a = layout.byID.get("a")!;
    const label = edgeLabelPoint(a, layout.byID.get("c")!);
    expect(label.x).toBeGreaterThan(a.x + nodeWidth);
    expect(label.x).toBeLessThan(layout.byID.get("b")!.x);
  });

  it("stays finite for a call that goes backwards", () => {
    const layout = layoutTopology([node("a"), node("b")], [edge("a", "b"), edge("b", "a")]);
    const back = edgeLabelPoint(layout.byID.get("b")!, layout.byID.get("a")!);
    expect(Number.isFinite(back.x) && Number.isFinite(back.y)).toBe(true);
  });
});
