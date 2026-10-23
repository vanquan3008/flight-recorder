import { useMemo } from "react";
import type { TopologyEdge, TopologyNode } from "./types";
import { edgePath, layoutTopology, nodeHeight, nodeWidth, truncate } from "./graph-layout";
import "./service-graph.css";

type Props = { nodes: TopologyNode[]; edges: TopologyEdge[]; selected: string; onSelect: (id: string) => void; label?: string };

/** Left-to-right caller → callee graph shared by the topology page and the per-trace service flow. */
export function ServiceGraph({ nodes, edges, selected, onSelect, label = "Service graph" }: Props) {
  const layout = useMemo(() => layoutTopology(nodes, edges), [nodes, edges]);
  const maxCalls = Math.max(1, ...edges.map((edge) => edge.call_count));
  const related = (edge: TopologyEdge) => edge.source_node_id === selected || edge.target_node_id === selected;
  if (!layout.nodes.length) return null;
  return <div className="graph-scroll" tabIndex={0} role="region" aria-label={label}>
    <svg className="graph-svg" width={layout.width} height={layout.height} viewBox={`0 0 ${layout.width} ${layout.height}`} role="group" aria-label={`${layout.nodes.length} nodes and ${edges.length} relationships`}>
      <defs>
        <marker id="graph-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="10" markerHeight="10" markerUnits="userSpaceOnUse" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" fill="context-stroke" /></marker>
      </defs>
      {edges.map((edge) => {
        const from = layout.byID.get(edge.source_node_id);
        const to = layout.byID.get(edge.target_node_id);
        if (!from || !to || from === to) return null;
        const failing = edge.error_count > 0;
        const dim = Boolean(selected) && !related(edge);
        return <path key={`${edge.source_node_id}>${edge.target_node_id}>${edge.kind}`} d={edgePath(from, to)} fill="none"
          className={`graph-edge${failing ? " failing" : ""}${dim ? " dim" : ""}`} strokeWidth={1.5 + (edge.call_count / maxCalls) * 3.5}
          strokeDasharray={failing ? "7 4" : undefined} markerEnd="url(#graph-arrow)">
          <title>{`${from.name} → ${to.name} · ${edge.kind} · ${edge.call_count} calls, ${edge.error_count} errors`}</title>
        </path>;
      })}
      {layout.nodes.map((node) => <g key={node.id} className={`graph-node type-${node.type.toLowerCase().replace(/[^a-z0-9]/g, "")}${selected === node.id ? " selected" : ""}`}
        transform={`translate(${node.x} ${node.y})`} role="button" tabIndex={0} aria-pressed={selected === node.id} aria-label={`${node.type} ${node.name}${node.isolated ? ", no observed calls" : ""}`}
        onClick={() => onSelect(selected === node.id ? "" : node.id)}
        onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); onSelect(selected === node.id ? "" : node.id); } }}>
        <rect width={nodeWidth} height={nodeHeight} rx="10" />
        <text className="graph-name" x="12" y="21">{truncate(node.name)}</text>
        <text className="graph-type" x="12" y="37">{node.isolated ? `${node.type} · no calls` : node.type}</text>
        <title>{node.name}</title>
      </g>)}
    </svg>
  </div>;
}
