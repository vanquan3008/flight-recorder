import { useId, useMemo, useState } from "react";
import type { TopologyEdge, TopologyNode } from "../types";
import { edgeLabelPoint, edgePath, layoutTopology, nodeHeight, nodeWidth, truncate } from "./graph-layout";
import { formatDuration } from "../ui";
import "./service-graph.css";

type Props = { nodes: TopologyNode[]; edges: TopologyEdge[]; selected: string; onSelect: (id: string) => void; label?: string };

type NodeStats = { callsIn: number; callsOut: number; errorsOut: number; errorsIn: number };

// Labels would collide on a busy graph, so a call shows its numbers only when focused or hovered.
const compact = new Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 });
const kindClass = (type: string) => type.toLowerCase().replace(/[^a-z0-9]/g, "");
const edgeKey = (edge: TopologyEdge) => `${edge.source_node_id}>${edge.target_node_id}>${edge.kind}`;
const plural = (value: number, noun: string) => `${value.toLocaleString()} ${noun}${value === 1 ? "" : "s"}`;

/** Left-to-right caller → callee graph shared by the topology page and the per-trace service flow. */
export function ServiceGraph({ nodes, edges, selected, onSelect, label = "Service graph" }: Props) {
  const id = useId().replace(/:/g, "");
  const [hovered, setHovered] = useState("");
  const layout = useMemo(() => layoutTopology(nodes, edges), [nodes, edges]);
  const stats = useMemo(() => {
    const byNode = new Map<string, NodeStats>();
    const entry = (key: string) => byNode.get(key) ?? byNode.set(key, { callsIn: 0, callsOut: 0, errorsOut: 0, errorsIn: 0 }).get(key)!;
    for (const edge of edges) {
      const source = entry(edge.source_node_id);
      source.callsOut += edge.call_count;
      source.errorsOut += edge.error_count;
      const target = entry(edge.target_node_id);
      target.callsIn += edge.call_count;
      target.errorsIn += edge.error_count;
    }
    return byNode;
  }, [edges]);
  const maxCalls = Math.max(1, ...edges.map((edge) => edge.call_count));
  const related = (edge: TopologyEdge) => edge.source_node_id === selected || edge.target_node_id === selected;
  const focus = useMemo(() => selected ? new Set([selected, ...edges.filter((edge) => edge.source_node_id === selected || edge.target_node_id === selected).flatMap((edge) => [edge.source_node_id, edge.target_node_id])]) : null, [edges, selected]);
  if (!layout.nodes.length) return null;
  const toggle = (nodeID: string) => onSelect(selected === nodeID ? "" : nodeID);
  return (
    <div className="graph-scroll" tabIndex={0} role="region" aria-label={label}>
      <svg className="graph-svg" style={{ maxWidth: layout.width }} viewBox={`0 0 ${layout.width} ${layout.height}`} role="group" aria-label={`${layout.nodes.length} nodes and ${edges.length} relationships`}>
        <defs>
          {(["ok", "bad"] as const).map((tone) => (
            <marker key={tone} id={`${id}-${tone}`} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="10" markerHeight="10" markerUnits="userSpaceOnUse" orient="auto">
              <path className={`graph-head ${tone}`} d="M0,0 L10,5 L0,10 z" />
            </marker>
          ))}
        </defs>
        {edges.map((edge) => {
          const from = layout.byID.get(edge.source_node_id);
          const to = layout.byID.get(edge.target_node_id);
          if (!from || !to || from === to) return null;
          const failing = edge.error_count > 0;
          const active = Boolean(selected) && related(edge);
          const dim = Boolean(selected) && !active;
          const path = edgePath(from, to);
          return (
            <g key={edgeKey(edge)} className={`graph-link${dim ? " dim" : ""}`}>
              <path d={path} fill="none" className={`graph-edge${failing ? " failing" : ""}${active ? " live" : ""}`}
                strokeWidth={1.4 + (edge.call_count / maxCalls) * 2.8}
                strokeDasharray={failing ? "7 4" : undefined}
                markerEnd={`url(#${id}-${failing ? "bad" : "ok"})`}>
                <title>{`${from.name} → ${to.name} · ${edge.kind} · ${plural(edge.call_count, "call")}, ${plural(edge.error_count, "error")} · avg ${formatDuration(edge.average_latency_nanos)} · p95 ${formatDuration(edge.p95_latency_nanos)}`}</title>
              </path>
              <path d={path} fill="none" className="graph-hit" onMouseEnter={() => setHovered(edgeKey(edge))} onMouseLeave={() => setHovered("")} />
            </g>
          );
        })}
        {layout.nodes.map((node) => {
          const own = stats.get(node.id) ?? { callsIn: 0, callsOut: 0, errorsOut: 0, errorsIn: 0 };
          const dim = focus !== null && !focus.has(node.id);
          const failing = own.errorsOut > 0;
          return (
            <g key={node.id}
              className={`graph-node type-${kindClass(node.type)}${selected === node.id ? " selected" : ""}${dim ? " dim" : ""}${failing ? " failing" : ""}`}
              transform={`translate(${node.x} ${node.y})`}
              role="button" tabIndex={0} aria-pressed={selected === node.id}
              aria-label={`${node.type} ${node.name}${node.isolated ? ", no observed calls" : ""}`}
              onClick={() => toggle(node.id)}
              onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); toggle(node.id); } }}>
              <rect className="graph-card" width={nodeWidth} height={nodeHeight} rx="14" />
              <rect className="graph-stripe" x="11" y="14" width="4" height={nodeHeight - 28} rx="2" />
              <text className="graph-name" x="26" y="26">{truncate(node.name, 20)}</text>
              <text className="graph-type" x="26" y="42">{node.type}</text>
              <text className="graph-stats" x="26" y="58">{node.isolated ? "No observed calls" : `in ${own.callsIn.toLocaleString()} · out ${own.callsOut.toLocaleString()}`}</text>
              {failing && <g className="graph-badge" transform={`translate(${nodeWidth - 68} 12)`}><rect width="56" height="18" rx="9" /><text x="28" y="12.5" textAnchor="middle">{own.errorsOut.toLocaleString()} err</text></g>}
              <title>{`${node.name} · ${node.type}\nCalls in ${own.callsIn.toLocaleString()} · calls out ${own.callsOut.toLocaleString()}\nErrors on calls out ${own.errorsOut.toLocaleString()} · errors on calls in ${own.errorsIn.toLocaleString()}`}</title>
            </g>
          );
        })}
        {edges.map((edge) => {
          const from = layout.byID.get(edge.source_node_id);
          const to = layout.byID.get(edge.target_node_id);
          const shown = (Boolean(selected) && related(edge)) || hovered === edgeKey(edge);
          if (!from || !to || from === to || !shown) return null;
          const failing = edge.error_count > 0;
          const first = `${compact.format(edge.call_count)} calls${failing ? ` · ${compact.format(edge.error_count)} err` : ""}`;
          const second = `avg ${formatDuration(edge.average_latency_nanos)}`;
          const labelWidth = Math.round(Math.max(first.length, second.length) * 6.2 + 18);
          const point = edgeLabelPoint(from, to);
          return (
            <g key={`label-${edgeKey(edge)}`} className="graph-label" transform={`translate(${point.x - labelWidth / 2} ${point.y - 17})`} aria-hidden="true">
              <rect width={labelWidth} height="34" rx="10" />
              <text x={labelWidth / 2} y="14" textAnchor="middle">{compact.format(edge.call_count)} calls{failing && <tspan className="err"> · {compact.format(edge.error_count)} err</tspan>}</text>
              <text className="sub" x={labelWidth / 2} y="27" textAnchor="middle">{second}</text>
            </g>
          );
        })}
      </svg>
    </div>
  );
}

/** What the colours and lines in ServiceGraph mean, for the node types that are actually drawn. */
export function GraphLegend({ nodes }: { nodes: TopologyNode[] }) {
  const types = [...new Set(nodes.map((node) => node.type))].sort();
  return (
    <div className="graph-key">
      <span className="graph-key-group">
        {types.map((type) => <span key={type}><i className={`graph-key-dot type-${kindClass(type)}`} aria-hidden="true" />{type}</span>)}
      </span>
      <span className="graph-key-group">
        <span><i className="graph-key-line" aria-hidden="true" />Calls · thicker is busier</span>
        <span><i className="graph-key-line bad" aria-hidden="true" />Calls with errors</span>
      </span>
      <span className="graph-key-hint">Select a node to focus its calls</span>
    </div>
  );
}
