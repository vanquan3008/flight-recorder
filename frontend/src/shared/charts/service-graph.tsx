import { useId, useMemo, useState } from "react";
import type { TopologyEdge, TopologyNode } from "../types";
import { edgeLabelPoint, edgePath, layoutTopology, nodeHeight, nodeWidth, truncate } from "./graph-layout";
import { formatDuration } from "../ui";

type Props = { nodes: TopologyNode[]; edges: TopologyEdge[]; selected: string; onSelect: (id: string) => void; label?: string };

type NodeStats = { callsIn: number; callsOut: number; errorsOut: number; errorsIn: number };

// Labels would collide on a busy graph, so a call shows its numbers only when focused or hovered.
const compact = new Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 });
const kindClass = (type: string) => type.toLowerCase().replace(/[^a-z0-9]/g, "");
function typeDotBg(type: string): string {
  const k = kindClass(type);
  if (k === "external") return "bg-violet";
  if (k === "database" || k === "db") return "bg-cyan";
  if (k === "messaging" || k === "queue") return "bg-warning";
  return "bg-accent";
}
function typeStripeFill(type: string): string {
  const k = kindClass(type);
  if (k === "external") return "fill-violet";
  if (k === "database" || k === "db") return "fill-cyan";
  if (k === "messaging" || k === "queue") return "fill-warning";
  return "fill-accent";
}
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
    <div className="graph-scroll relative flex min-h-[340px] h-full min-h-0 flex-1 overflow-auto rounded-none bg-glass-sunken bg-[radial-gradient(circle_at_1px_1px,color-mix(in_srgb,var(--text)_14%,transparent)_1px,transparent_0)] bg-size-[24px_24px] focus-visible:-outline-offset-2" tabIndex={0} role="region" aria-label={label}>
      <svg className="graph-svg m-auto block h-auto w-full flex-[0_1_auto] overflow-visible" style={{ maxWidth: layout.width }} viewBox={`0 0 ${layout.width} ${layout.height}`} role="group" aria-label={`${layout.nodes.length} nodes and ${edges.length} relationships`}>
        <defs>
          {(["ok", "bad"] as const).map((tone) => (
            <marker key={tone} id={`${id}-${tone}`} viewBox="0 0 12 12" refX="10" refY="6" markerWidth="10" markerHeight="10" markerUnits="userSpaceOnUse" orient="auto">
              <path className={`graph-head [&.ok]:fill-[color-mix(in_srgb,var(--accent)_75%,#ffffff)] [&.bad]:fill-danger ${tone}`} d="M 1 2 L 10 6 L 1 10 C 3 6 3 6 1 2 Z" />
            </marker>
          ))}
          <filter id={`${id}-glow-selected`} x="-30%" y="-30%" width="160%" height="160%">
            <feDropShadow dx="0" dy="0" stdDeviation="6" floodColor="var(--accent)" floodOpacity="0.45" />
          </filter>
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
            <g key={edgeKey(edge)} className={`graph-link transition-opacity ${dim ? "opacity-15 dim" : ""}`}>
              {/* Main connecting stroke */}
              <path d={path} fill="none" className={`graph-edge stroke-[color-mix(in_srgb,var(--accent)_65%,var(--faint))] opacity-90 transition-all [stroke-linecap:round] ${failing ? "stroke-danger failing" : ""} ${active ? "[stroke-dasharray:8_6] motion-safe:animate-graph-flow live" : ""} ${failing && active ? "[stroke-dasharray:7_4]" : ""}`}
                strokeWidth={1.8 + (edge.call_count / maxCalls) * 2.8}
                markerEnd={`url(#${id}-${failing ? "bad" : "ok"})`}>
                <title>{`${from.name} → ${to.name} · ${edge.kind} · ${plural(edge.call_count, "call")}, ${plural(edge.error_count, "error")} · avg ${formatDuration(edge.average_latency_nanos)} · p95 ${formatDuration(edge.p95_latency_nanos)}`}</title>
              </path>
              {/* Telemetry flow particle dash */}
              {(active || !dim) && (
                <path d={path} fill="none" className={`pointer-events-none opacity-60 animate-[graph-flow_1s_linear_infinite] [stroke-linecap:round] ${failing ? "stroke-danger-fg" : "stroke-[color-mix(in_srgb,var(--accent)_85%,#ffffff)]"}`}
                  strokeWidth={1.8 + (edge.call_count / maxCalls) * 2.8}
                  strokeDasharray={failing ? "5 9" : "6 10"} />
              )}
              <path d={path} fill="none" className="graph-hit stroke-transparent stroke-[18px] [pointer-events:stroke]" onMouseEnter={() => setHovered(edgeKey(edge))} onMouseLeave={() => setHovered("")} />
            </g>
          );
        })}
        {layout.nodes.map((node) => {
          const own = stats.get(node.id) ?? { callsIn: 0, callsOut: 0, errorsOut: 0, errorsIn: 0 };
          const dim = focus !== null && !focus.has(node.id);
          const failing = own.errorsOut > 0;
          return (
            <g key={node.id}
              className={`graph-node cursor-pointer outline-none transition-transform duration-150 hover:-translate-y-0.5 focus-visible:-translate-y-0.5 ${dim ? "opacity-35 dim" : ""} [&:hover_.graph-card]:stroke-accent [&:focus-visible_.graph-card]:stroke-accent aria-pressed:[&_.graph-card]:stroke-accent aria-pressed:[&_.graph-card]:stroke-2 aria-pressed:[&_.graph-card]:fill-[color-mix(in_srgb,var(--accent)_14%,var(--surface-solid))] ${failing && selected !== node.id ? "[&_.graph-card]:stroke-danger/45" : ""} type-${kindClass(node.type)}${selected === node.id ? " selected" : ""}${failing ? " failing" : ""}`}
              transform={`translate(${node.x} ${node.y})`}
              role="button" tabIndex={0} aria-pressed={selected === node.id}
              aria-label={`${node.type} ${node.name}${node.isolated ? ", no observed calls" : ""}`}
              onClick={() => toggle(node.id)}
              onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); toggle(node.id); } }}>
              {/* Outer halo when selected */}
              {selected === node.id && (
                <rect x="-4" y="-4" width={nodeWidth + 8} height={nodeHeight + 8} rx="16" fill="none" className="stroke-accent stroke-[1.8px] opacity-75" filter={`url(#${id}-glow-selected)`} />
              )}
              {/* Main Card Shell */}
              <rect className="graph-card fill-surface-solid stroke-glass-field-edge stroke-[1.2px] drop-shadow-md transition-[stroke,fill,filter]" width={nodeWidth} height={nodeHeight} rx="14" />
              {/* Left colored stripe indicator */}
              <rect className={`graph-stripe ${typeStripeFill(node.type)}`} x="10" y="14" width="4" height={nodeHeight - 28} rx="2" />
              {/* Node Title */}
              <text className="graph-name fill-text text-[13.5px] font-bold tracking-tight" x="22" y="27">{truncate(node.name, 16)}</text>
              {/* Node Type Pill */}
              <g transform={`translate(${nodeWidth - 58} 9)`}>
                <rect width="48" height="15" rx="7.5" className="fill-glass-field stroke-glass-divider stroke-[0.8]" />
                <text x="24" y="11" textAnchor="middle" className="graph-type fill-faint font-mono text-[8.5px] font-bold tracking-wider uppercase">{truncate(node.type, 7)}</text>
              </g>
              {/* Bottom stats capsule */}
              <rect x="20" y="38" width={nodeWidth - 28} height="22" rx="6" className="fill-glass-field stroke-glass-divider stroke-[0.8]" />
              <text className="graph-stats fill-muted font-mono text-[10.5px] font-medium tabular-nums" x="27" y="53">{node.isolated ? "No observed calls" : `in ${own.callsIn.toLocaleString()} · out ${own.callsOut.toLocaleString()}`}</text>
              {/* Health Indicator: green dot if healthy */}
              {!failing && !node.isolated && (
                <g transform={`translate(${nodeWidth - 16} 49)`}>
                  <circle r="4" className="fill-emerald-500/20" />
                  <circle r="2" className="fill-emerald-500" />
                </g>
              )}
              {/* Error Badge if failing */}
              {failing && (
                <g className="graph-badge" transform={`translate(${nodeWidth - 68} 8)`}>
                  <rect width="60" height="17" rx="8.5" className="fill-danger-bg stroke-danger/40 stroke-1" />
                  <circle cx="8" cy="8.5" r="2.5" className="fill-danger-fg animate-pulse" />
                  <text x="34" y="12" textAnchor="middle" className="font-mono text-[9.5px] font-bold fill-danger-fg">{own.errorsOut.toLocaleString()} err</text>
                </g>
              )}
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
          const labelWidth = Math.round(Math.max(first.length, second.length) * 6.4 + 24);
          const point = edgeLabelPoint(from, to);
          return (
            <g key={`label-${edgeKey(edge)}`} className="graph-label pointer-events-none" transform={`translate(${point.x - labelWidth / 2} ${point.y - 19})`} aria-hidden="true">
              <rect width={labelWidth} height="38" rx="12" className="fill-surface-solid stroke-glass-field-edge stroke-[1.2px] shadow-xl" />
              <text x={labelWidth / 2} y="15" textAnchor="middle" className="font-mono text-[11px] font-semibold fill-text">
                {compact.format(edge.call_count)} calls{failing && <tspan className="err fill-danger-fg font-bold"> · {compact.format(edge.error_count)} err</tspan>}
              </text>
              <text className="sub font-mono text-[10px] font-medium fill-muted" x={labelWidth / 2} y="30" textAnchor="middle">
                {second}
              </text>
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
    <div className="graph-key flex flex-wrap items-center gap-x-5.5 gap-y-2 border-t border-glass-divider bg-glass-field/30 px-4.5 py-2.5 text-xs text-muted">
      <span className="graph-key-group inline-flex items-center gap-2 [&>span]:inline-flex [&>span]:items-center [&>span]:gap-1.5 [&>span]:rounded-full [&>span]:border [&>span]:border-glass-divider [&>span]:bg-glass-field [&>span]:px-2.5 [&>span]:py-0.5 [&>span]:font-mono [&>span]:text-[10px]">
        {types.map((type) => <span key={type}><i className={`graph-key-dot size-2 rounded-full ${typeDotBg(type)} type-${kindClass(type)}`} aria-hidden="true" />{type}</span>)}
      </span>
      <span className="graph-key-group inline-flex items-center gap-4 text-[11px] [&>span]:inline-flex [&>span]:items-center [&>span]:gap-1.75">
        <span><i className="graph-key-line h-0 w-5.5 border-t-2 border-accent/60" aria-hidden="true" />Calls · thicker is busier</span>
        <span><i className="graph-key-line h-0 w-5.5 border-t-2 border-danger border-dashed" aria-hidden="true" />Calls with errors</span>
      </span>
      <span className="graph-key-hint ml-auto font-mono text-[10.5px] text-faint">Select a node to focus its calls</span>
    </div>
  );
}
