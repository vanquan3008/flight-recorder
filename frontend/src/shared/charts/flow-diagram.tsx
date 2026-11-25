import { useId } from "react";
import type { KeyboardEvent } from "react";
import type { TopologyEdge, TopologyNode } from "../types";
import { truncate } from "./graph-layout";
import { kindClass, plural, typeStripeFill } from "./node-style";

export type FlowLink = { edge: TopologyEdge; node: TopologyNode };

const nodeWidth = 186;
const nodeHeight = 58;
const rowGap = 16;
const centerWidth = 224;
const centerHeight = 80;
const columnGap = 150;
const top = 40;
const maxPerSide = 8;
const width = nodeWidth * 2 + centerWidth + columnGap * 2;
const centerX = nodeWidth + columnGap;
const rightX = centerX + centerWidth + columnGap;


/** A service in the middle, the services that call it on the left and what it calls on the right. */
export function FlowDiagram({ node, callers, callees, onSelect }: { node: TopologyNode; callers: FlowLink[]; callees: FlowLink[]; onSelect: (id: string) => void }) {
  const id = useId().replace(/:/g, "");
  const rows = Math.min(maxPerSide, Math.max(callers.length, callees.length, 1));
  const bodyHeight = Math.max(rows * nodeHeight + (rows - 1) * rowGap, centerHeight);
  const hidden = Math.max(callers.length, callees.length) > maxPerSide;
  const height = top + bodyHeight + (hidden ? 28 : 16);
  const middle = top + bodyHeight / 2;
  const maxCalls = Math.max(1, ...[...callers, ...callees].map((link) => link.edge.call_count));

  const side = (links: FlowLink[], column: "left" | "right") => {
    const shown = links.slice(0, maxPerSide);
    const blockHeight = shown.length * nodeHeight + (shown.length - 1) * rowGap;
    const startY = top + (bodyHeight - blockHeight) / 2;
    const x = column === "left" ? 0 : rightX;
    return (
      <>
        {shown.length === 0 && (
          <g className="fd-empty-slot" transform={`translate(${x} ${middle - nodeHeight / 2})`}>
            <rect width={nodeWidth} height={nodeHeight} rx="14" fill="none"
              className="stroke-glass-divider stroke-[1.2] [stroke-dasharray:4_4] opacity-75" />
            <circle cx="24" cy={nodeHeight / 2} r="3" className="fill-faint opacity-40" />
            <text className="fd-empty fill-faint font-mono text-[11px] font-medium tracking-wide" x={nodeWidth / 2 + 8} y={nodeHeight / 2 + 4} textAnchor="middle">
              None observed
            </text>
          </g>
        )}
        {shown.map((link, index) => {
          const y = startY + index * (nodeHeight + rowGap);
          const failing = link.edge.error_count > 0;
          const spreadStep = shown.length > 1 ? Math.min(16, (centerHeight - 28) / (shown.length - 1)) : 0;
          const exitY = middle + (shown.length > 1 ? (index - (shown.length - 1) / 2) * spreadStep : 0);
          const from = column === "left" ? { x: nodeWidth, y: y + nodeHeight / 2 } : { x: centerX + centerWidth, y: exitY };
          const to = column === "left" ? { x: centerX - 2, y: exitY } : { x: rightX - 2, y: y + nodeHeight / 2 };
          const bend = columnGap / 2;
          const labelX = (from.x + to.x) / 2;
          const labelY = (from.y + to.y) / 2;
          const label = plural(link.edge.call_count, "call");
          const labelWidth = failing ? 104 : 68;
          const select = () => onSelect(link.node.id);
          const onKey = (event: KeyboardEvent) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); select(); } };
          const edgeCurve = `M ${from.x} ${from.y} C ${from.x + bend} ${from.y}, ${to.x - bend} ${to.y}, ${to.x} ${to.y}`;
          return (
            <g key={`${link.edge.source_node_id}>${link.edge.target_node_id}>${link.edge.kind}`} className="fd-link-group">
              {/* Base SVG connecting edge */}
              <path className={`fd-edge fill-none stroke-[color-mix(in_srgb,var(--accent)_65%,var(--faint))] opacity-90 transition-all [stroke-linecap:round] ${failing ? "stroke-danger failing" : ""}`}
                d={edgeCurve}
                strokeWidth={1.8 + (link.edge.call_count / maxCalls) * 1.8}
                markerEnd={`url(#${id}-${failing ? "bad" : "ok"})`} />
              {/* Animated data flow pulse */}
              <path className={`pointer-events-none fill-none opacity-60 animate-[graph-flow_1s_linear_infinite] [stroke-linecap:round] ${failing ? "stroke-danger-fg" : "stroke-[color-mix(in_srgb,var(--accent)_85%,#ffffff)]"}`}
                d={edgeCurve}
                strokeWidth={1.8 + (link.edge.call_count / maxCalls) * 1.8}
                strokeDasharray={failing ? "5 9" : "6 10"} />
              {/* Telemetry Edge Label Badge */}
              <g className="fd-label [&>rect]:fill-surface-solid [&>rect]:stroke-glass-field-edge [&>text]:fill-text [&>text]:text-[10.5px] [&>text]:font-semibold [&_.err]:fill-danger-fg [&_.err]:font-bold" transform={`translate(${labelX - labelWidth / 2} ${labelY - 12})`}>
                <rect width={labelWidth} height="24" rx="12" className="shadow-md" />
                <text x={labelWidth / 2} y="16" textAnchor="middle" className="font-mono">{label}{failing && <tspan className="err"> · {link.edge.error_count} err</tspan>}</text>
              </g>
              {/* Interactive Node Card */}
              <g className="fd-node group cursor-pointer outline-none transition-transform duration-150 hover:-translate-y-0.5 focus-visible:-translate-y-0.5 link"
                transform={`translate(${x} ${y})`} role="button" tabIndex={0} onClick={select} onKeyDown={onKey}
                aria-label={`${link.node.name}, ${link.node.type}, ${plural(link.edge.call_count, "call")}${failing ? `, ${plural(link.edge.error_count, "error")}` : ""}`}>
                <rect width={nodeWidth} height={nodeHeight} rx="14" filter={`url(#${id}-card-shadow)`}
                  className="fill-surface-solid stroke-glass-field-edge stroke-[1.2px] transition-[stroke,fill] group-hover:stroke-accent group-hover:fill-glass-field-focus group-focus-visible:stroke-accent" />
                {/* Node type colored stripe */}
                <rect x="10" y="15" width="3.5" height={nodeHeight - 30} rx="1.75"
                  className={`${typeStripeFill(link.node.type)} type-${kindClass(link.node.type)}`} />
                {/* Status indicator ring and dot */}
                <g transform={`translate(25 ${nodeHeight / 2})`}>
                  <circle r={failing ? 6 : 5} className={failing ? "fill-danger-bg animate-pulse" : "fill-ok-bg"} />
                  <circle r="2.8" className={failing ? "fill-danger-fg" : "fill-ok-fg"} />
                </g>
                <text className="fd-name fill-text text-[13px] font-bold tracking-tight" x="38" y="25">{truncate(link.node.name, 14)}</text>
                {/* Type mini tag */}
                <g transform={`translate(${nodeWidth - 58} 9)`}>
                  <rect width="48" height="15" rx="7.5" className="fill-glass-field stroke-glass-divider stroke-[0.8]" />
                  <text x="24" y="11" textAnchor="middle" className="fill-faint font-mono text-[8.5px] font-bold tracking-wider uppercase">{truncate(link.node.type, 7)}</text>
                </g>
                <text className="fd-type fill-muted font-mono text-[10.5px] font-medium" x="38" y="45">
                  {plural(link.edge.call_count, "call")}
                  {failing ? <tspan className="err fill-danger-fg font-bold"> · {link.edge.error_count} err</tspan> : <tspan className="fill-ok-fg"> · ok</tspan>}
                </text>
              </g>
            </g>
          );
        })}
        {links.length > shown.length && (
          <text className="fd-empty fill-faint font-mono text-xs" x={x + nodeWidth / 2} y={top + bodyHeight + 18} textAnchor="middle">
            +{links.length - shown.length} more in Call details
          </text>
        )}
      </>
    );
  };

  return (
    <svg className="flow-diagram mx-auto block h-auto w-full max-w-[940px] overflow-visible" viewBox={`0 0 ${width} ${height}`} width="100%"
      aria-label={`${node.name} with ${plural(callers.length, "caller")} and ${plural(callees.length, "callee")}`}>
      <defs>
        {(["ok", "bad"] as const).map((tone) => (
          <marker key={tone} id={`${id}-${tone}`} viewBox="0 0 12 12" refX="10" refY="6" markerWidth="9" markerHeight="9" markerUnits="userSpaceOnUse" orient="auto">
            <path className={`fd-head [&.ok]:fill-[color-mix(in_srgb,var(--accent)_75%,#ffffff)] [&.bad]:fill-danger ${tone}`} d="M 1 2 L 10 6 L 1 10 C 3 6 3 6 1 2 Z" />
          </marker>
        ))}
        <filter id={`${id}-card-shadow`} x="-20%" y="-20%" width="140%" height="140%">
          <feDropShadow dx="0" dy="3" stdDeviation="4" floodColor="#000000" floodOpacity="0.08" />
        </filter>
        <filter id={`${id}-glow-focus`} x="-30%" y="-30%" width="160%" height="160%">
          <feDropShadow dx="0" dy="0" stdDeviation="8" floodColor="var(--accent)" floodOpacity="0.45" />
        </filter>
      </defs>
      {/* Column Headers as sleek telemetry badges */}
      <g transform={`translate(${nodeWidth / 2 - 64} 10)`}>
        <rect width="128" height="24" rx="12" className="fill-surface-solid stroke-glass-divider stroke-[1.2px] shadow-sm" />
        <circle cx="16" cy="12" r="3" className="fill-faint" />
        <text x="70" y="16" textAnchor="middle" className="fd-column fill-muted font-mono text-[9.5px] font-bold tracking-wider uppercase">Called by · {callers.length}</text>
      </g>
      <g transform={`translate(${centerX + centerWidth / 2 - 68} 10)`}>
        <rect width="136" height="24" rx="12" className="fill-accent/15 stroke-accent/40 stroke-[1.2px] shadow-sm" />
        <circle cx="17" cy="12" r="3" className="fill-accent" />
        <text x="74" y="16" textAnchor="middle" className="fd-column fill-accent font-mono text-[9.5px] font-bold tracking-wider uppercase">Target Service</text>
      </g>
      <g transform={`translate(${rightX + nodeWidth / 2 - 64} 10)`}>
        <rect width="128" height="24" rx="12" className="fill-surface-solid stroke-glass-divider stroke-[1.2px] shadow-sm" />
        <circle cx="16" cy="12" r="3" className="fill-faint" />
        <text x="70" y="16" textAnchor="middle" className="fd-column fill-muted font-mono text-[9.5px] font-bold tracking-wider uppercase">Calls out · {callees.length}</text>
      </g>
      {side(callers, "left")}
      {side(callees, "right")}
      {/* Central Focused Node */}
      <g className="fd-node self" transform={`translate(${centerX} ${middle - centerHeight / 2})`}>
        {/* Glow halo */}
        <rect width={centerWidth + 8} height={centerHeight + 8} rx="18" x="-4" y="-4" fill="none" className="stroke-accent stroke-[1.5px] opacity-60" filter={`url(#${id}-glow-focus)`} />
        {/* Main card */}
        <rect width={centerWidth} height={centerHeight} rx="16" className="fill-[color-mix(in_srgb,var(--accent)_14%,var(--surface-solid))] stroke-accent stroke-2 shadow-xl" />
        {/* Top Header */}
        <g transform="translate(18 20)">
          <circle r="7" className="fill-accent/20 animate-ping opacity-60" />
          <circle r="3.5" className="fill-accent" />
        </g>
        <text className="fd-type fill-accent font-mono text-[10px] font-bold tracking-widest uppercase" x="32" y="23">{node.type}</text>
        <g transform={`translate(${centerWidth - 56} 12)`}>
          <rect width="44" height="16" rx="8" className="fill-accent/25 stroke-accent/50 stroke-[0.8]" />
          <text x="22" y="11.5" textAnchor="middle" className="fill-accent font-mono text-[8.5px] font-bold tracking-wider uppercase">FOCUS</text>
        </g>
        {/* Service Name */}
        <text className="fd-name fill-text text-[15.5px] font-bold tracking-tight big" x={centerWidth / 2} y="47" textAnchor="middle">{truncate(node.name, 20)}</text>
        {/* Subtitle Summary */}
        <text className="fill-muted font-mono text-[10.5px] font-semibold" x={centerWidth / 2} y="66" textAnchor="middle">{callers.length} in · {callees.length} out</text>
      </g>
    </svg>
  );
}
