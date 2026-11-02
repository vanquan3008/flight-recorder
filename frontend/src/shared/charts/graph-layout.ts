import type { TopologyEdge, TopologyNode } from "../types";

export const nodeWidth = 192;
export const nodeHeight = 68;
export const columnGap = 124;
const rowGap = 30;
const margin = 24;
const bandGap = 56;

export type PlacedNode = TopologyNode & { x: number; y: number; layer: number; isolated: boolean };
export type TopologyLayout = { nodes: PlacedNode[]; width: number; height: number; byID: Map<string, PlacedNode> };

/**
 * Layered left-to-right layout: callers sit left of their callees. Cycles are tolerated
 * (members of a cycle are placed after their already-placed callers), and services with
 * no observed relationship go into a separate band so they never distort the graph.
 */
export function layoutTopology(nodes: TopologyNode[], edges: TopologyEdge[]): TopologyLayout {
  const ids = new Set(nodes.map((node) => node.id));
  const valid = edges.filter((edge) => ids.has(edge.source_node_id) && ids.has(edge.target_node_id) && edge.source_node_id !== edge.target_node_id);
  const connected = new Set(valid.flatMap((edge) => [edge.source_node_id, edge.target_node_id]));
  const outgoing = new Map<string, string[]>();
  const incoming = new Map<string, string[]>();
  const indegree = new Map<string, number>();
  for (const edge of valid) {
    outgoing.set(edge.source_node_id, [...(outgoing.get(edge.source_node_id) ?? []), edge.target_node_id]);
    incoming.set(edge.target_node_id, [...(incoming.get(edge.target_node_id) ?? []), edge.source_node_id]);
    indegree.set(edge.target_node_id, (indegree.get(edge.target_node_id) ?? 0) + 1);
  }

  const layer = new Map<string, number>();
  const queue = [...connected].filter((id) => !indegree.get(id)).sort();
  queue.forEach((id) => layer.set(id, 0));
  const remaining = new Map(indegree);
  while (queue.length) {
    const id = queue.shift()!;
    for (const target of outgoing.get(id) ?? []) {
      layer.set(target, Math.max(layer.get(target) ?? 0, (layer.get(id) ?? 0) + 1));
      remaining.set(target, (remaining.get(target) ?? 1) - 1);
      if (remaining.get(target) === 0) queue.push(target);
    }
  }
  // Cycle members never reach in-degree zero; place them after the callers that were placed.
  for (const id of [...connected].sort()) {
    if (layer.has(id)) continue;
    const placed = (incoming.get(id) ?? []).filter((source) => layer.has(source));
    layer.set(id, placed.length ? Math.max(...placed.map((source) => layer.get(source)!)) + 1 : 0);
  }

  const byName = (left: TopologyNode, right: TopologyNode) => left.name.localeCompare(right.name) || left.id.localeCompare(right.id);
  const columns: TopologyNode[][] = [];
  for (const node of nodes.filter((item) => connected.has(item.id)).sort(byName)) {
    const index = layer.get(node.id) ?? 0;
    (columns[index] ??= []).push(node);
  }
  // One barycenter pass keeps related nodes close and reduces edge crossings without a solver.
  const row = new Map<string, number>();
  columns.forEach((column, index) => {
    if (index > 0) {
      const weight = (node: TopologyNode) => {
        const sources = (incoming.get(node.id) ?? []).filter((source) => row.has(source));
        return sources.length ? sources.reduce((sum, source) => sum + row.get(source)!, 0) / sources.length : Number.MAX_SAFE_INTEGER;
      };
      column.sort((left, right) => weight(left) - weight(right) || byName(left, right));
    }
    column.forEach((node, position) => row.set(node.id, position));
  });

  const placed: PlacedNode[] = [];
  const tallest = Math.max(0, ...columns.map((column) => column?.length ?? 0));
  const graphHeight = tallest ? tallest * (nodeHeight + rowGap) - rowGap : 0;
  columns.forEach((column, index) => (column ?? []).forEach((node, position) => {
    const offset = (graphHeight - (column.length * (nodeHeight + rowGap) - rowGap)) / 2;
    placed.push({ ...node, layer: index, isolated: false, x: margin + index * (nodeWidth + columnGap), y: margin + offset + position * (nodeHeight + rowGap) });
  }));

  const isolated = nodes.filter((node) => !connected.has(node.id)).sort(byName);
  const graphWidth = Math.max(columns.length * (nodeWidth + columnGap) - columnGap, 0) + margin * 2;
  const perRow = Math.max(1, Math.floor(Math.max(graphWidth, nodeWidth + margin * 2) / (nodeWidth + columnGap / 2)));
  const bandTop = margin + graphHeight + (graphHeight && isolated.length ? bandGap : 0);
  isolated.forEach((node, position) => placed.push({
    ...node, layer: -1, isolated: true,
    x: margin + (position % perRow) * (nodeWidth + columnGap / 2),
    y: bandTop + Math.floor(position / perRow) * (nodeHeight + rowGap),
  }));

  const width = Math.max(graphWidth, margin * 2 + Math.min(isolated.length, perRow) * (nodeWidth + columnGap / 2));
  const height = isolated.length ? bandTop + Math.ceil(isolated.length / perRow) * (nodeHeight + rowGap) - rowGap + margin : graphHeight + margin * 2;
  return { nodes: placed, width: Math.max(width, 1), height: Math.max(height, 1), byID: new Map(placed.map((node) => [node.id, node])) };
}

export function edgePath(from: PlacedNode, to: PlacedNode): string {
  const x1 = from.x + nodeWidth;
  const y1 = from.y + nodeHeight / 2;
  const x2 = to.x;
  const y2 = to.y + nodeHeight / 2;
  if (to.layer > from.layer) {
    const bend = Math.max((x2 - x1) / 2, 24);
    return `M${x1},${y1} C${x1 + bend},${y1} ${x2 - bend},${y2} ${x2},${y2}`;
  }
  // Same-column or backwards call: leave from the right and re-enter from the left over the top.
  const lift = Math.min(from.y, to.y) - 18;
  return `M${x1},${y1} C${x1 + 60},${lift} ${x2 - 60},${lift} ${x2},${y2}`;
}

/**
 * Where to put a label on the curve drawn by edgePath: the point above the middle of the gap after
 * the caller. For neighbouring columns that is the middle of the curve; for a call that skips
 * columns it keeps the label out of the nodes in between.
 */
export function edgeLabelPoint(from: PlacedNode, to: PlacedNode): { x: number; y: number } {
  const x1 = from.x + nodeWidth;
  const y1 = from.y + nodeHeight / 2;
  const x2 = to.x;
  const y2 = to.y + nodeHeight / 2;
  let c1 = { x: x1 + 60, y: Math.min(from.y, to.y) - 18 };
  let c2 = { x: x2 - 60, y: c1.y };
  if (to.layer > from.layer) {
    const bend = Math.max((x2 - x1) / 2, 24);
    c1 = { x: x1 + bend, y: y1 };
    c2 = { x: x2 - bend, y: y2 };
  }
  const at = (t: number) => {
    const u = 1 - t;
    return {
      x: u * u * u * x1 + 3 * u * u * t * c1.x + 3 * u * t * t * c2.x + t * t * t * x2,
      y: u * u * u * y1 + 3 * u * u * t * c1.y + 3 * u * t * t * c2.y + t * t * t * y2,
    };
  };
  const wanted = x1 + columnGap / 2;
  let best = at(0.5);
  let distance = Infinity;
  for (let step = 0; step <= 50; step++) {
    const point = at(step / 50);
    const gap = Math.abs(point.x - wanted);
    if (gap < distance) { distance = gap; best = point; }
  }
  return best;
}

export function truncate(value: string, limit = 22): string {
  return value.length > limit ? `${value.slice(0, limit - 1)}…` : value;
}
