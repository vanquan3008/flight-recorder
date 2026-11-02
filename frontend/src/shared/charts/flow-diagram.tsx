import { useId } from "react";
import type { KeyboardEvent } from "react";
import type { TopologyEdge, TopologyNode } from "../types";
import "./flow-diagram.css";

export type FlowLink = { edge: TopologyEdge; node: TopologyNode };

const nodeWidth = 170;
const nodeHeight = 50;
const rowGap = 14;
const centerWidth = 190;
const centerHeight = 64;
const columnGap = 170;
const top = 26;
const maxPerSide = 8;
const width = nodeWidth * 2 + centerWidth + columnGap * 2;
const centerX = nodeWidth + columnGap;
const rightX = centerX + centerWidth + columnGap;

const truncate = (text: string, limit: number) => (text.length > limit ? `${text.slice(0, limit - 1)}…` : text);
const plural = (value: number, noun: string) => `${value} ${noun}${value === 1 ? "" : "s"}`;
const kindClass = (type: string) => type.toLowerCase().replace(/[^a-z0-9]/g, "");

/** A service in the middle, the services that call it on the left and what it calls on the right. */
export function FlowDiagram({ node, callers, callees, onSelect }: { node: TopologyNode; callers: FlowLink[]; callees: FlowLink[]; onSelect: (id: string) => void }) {
  const id = useId().replace(/:/g, "");
  const rows = Math.min(maxPerSide, Math.max(callers.length, callees.length, 1));
  const bodyHeight = rows * nodeHeight + (rows - 1) * rowGap;
  const hidden = Math.max(callers.length, callees.length) > maxPerSide;
  const height = top + bodyHeight + (hidden ? 22 : 8);
  const middle = top + bodyHeight / 2;
  const maxCalls = Math.max(1, ...[...callers, ...callees].map((link) => link.edge.call_count));

  const side = (links: FlowLink[], column: "left" | "right") => {
    const shown = links.slice(0, maxPerSide);
    const blockHeight = shown.length * nodeHeight + (shown.length - 1) * rowGap;
    const startY = top + (bodyHeight - blockHeight) / 2;
    const x = column === "left" ? 0 : rightX;
    return (
      <>
        {shown.length === 0 && <text className="fd-empty" x={x + nodeWidth / 2} y={middle + 4} textAnchor="middle">None observed</text>}
        {shown.map((link, index) => {
          const y = startY + index * (nodeHeight + rowGap);
          const failing = link.edge.error_count > 0;
          const from = column === "left" ? { x: nodeWidth, y: y + nodeHeight / 2 } : { x: centerX + centerWidth, y: middle };
          const to = column === "left" ? { x: centerX - 2, y: middle } : { x: rightX - 2, y: y + nodeHeight / 2 };
          const bend = columnGap / 2;
          const labelX = (from.x + to.x) / 2;
          const labelY = (from.y + to.y) / 2;
          const label = plural(link.edge.call_count, "call");
          const labelWidth = failing ? 98 : 64;
          const select = () => onSelect(link.node.id);
          const onKey = (event: KeyboardEvent) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); select(); } };
          return (
            <g key={`${link.edge.source_node_id}>${link.edge.target_node_id}>${link.edge.kind}`}>
              <path className={`fd-edge${failing ? " failing" : ""}`}
                d={`M ${from.x} ${from.y} C ${from.x + bend} ${from.y}, ${to.x - bend} ${to.y}, ${to.x} ${to.y}`}
                strokeWidth={1.2 + (link.edge.call_count / maxCalls) * 1.6}
                markerEnd={`url(#${id}-${failing ? "bad" : "ok"})`} />
              <g className="fd-label" transform={`translate(${labelX - labelWidth / 2} ${labelY - 10})`}>
                <rect width={labelWidth} height="20" rx="10" />
                <text x={labelWidth / 2} y="14" textAnchor="middle">{label}{failing && <tspan className="err"> · {link.edge.error_count} err</tspan>}</text>
              </g>
              <g className="fd-node link" transform={`translate(${x} ${y})`} role="button" tabIndex={0} onClick={select} onKeyDown={onKey}
                aria-label={`${link.node.name}, ${link.node.type}, ${plural(link.edge.call_count, "call")}${failing ? `, ${plural(link.edge.error_count, "error")}` : ""}`}>
                <rect width={nodeWidth} height={nodeHeight} rx="12" />
                <circle className={`fd-dot ${kindClass(link.node.type)}`} cx="16" cy={nodeHeight / 2} r="4" />
                <text className="fd-name" x="30" y="22">{truncate(link.node.name, 16)}</text>
                <text className="fd-type" x="30" y="38">{link.node.type}</text>
              </g>
            </g>
          );
        })}
        {links.length > shown.length && (
          <text className="fd-empty" x={x + nodeWidth / 2} y={top + bodyHeight + 16} textAnchor="middle">
            +{links.length - shown.length} more in Call details
          </text>
        )}
      </>
    );
  };

  return (
    <svg className="flow-diagram" viewBox={`0 0 ${width} ${height}`} width="100%"
      aria-label={`${node.name} with ${plural(callers.length, "caller")} and ${plural(callees.length, "callee")}`}>
      <defs>
        {(["ok", "bad"] as const).map((tone) => (
          <marker key={tone} id={`${id}-${tone}`} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="9" markerHeight="9" markerUnits="userSpaceOnUse" orient="auto">
            <path className={`fd-head ${tone}`} d="M 0 0 L 10 5 L 0 10 z" />
          </marker>
        ))}
      </defs>
      <text className="fd-column" x={nodeWidth / 2} y="12" textAnchor="middle">Called by</text>
      <text className="fd-column" x={centerX + centerWidth / 2} y="12" textAnchor="middle">Service</text>
      <text className="fd-column" x={rightX + nodeWidth / 2} y="12" textAnchor="middle">Calls out</text>
      {side(callers, "left")}
      {side(callees, "right")}
      <g className="fd-node self" transform={`translate(${centerX} ${middle - centerHeight / 2})`}>
        <rect width={centerWidth} height={centerHeight} rx="14" />
        <text className="fd-name big" x={centerWidth / 2} y="28" textAnchor="middle">{truncate(node.name, 22)}</text>
        <text className="fd-type" x={centerWidth / 2} y="46" textAnchor="middle">{node.type}</text>
      </g>
    </svg>
  );
}
