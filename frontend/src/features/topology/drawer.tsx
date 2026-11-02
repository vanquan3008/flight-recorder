import { X } from "lucide-react";
import { Link } from "react-router-dom";
import type { UseQueryResult } from "@tanstack/react-query";
import type { FlowLink } from "../../shared/charts";
import type { LogRecord, Page, TopologyNode } from "../../shared/types";
import { Badge, DetailLink, Empty, ErrorState, Loading, formatDuration, formatTime, logBodyText } from "../../shared/ui";

const shownLogs = 8;
const count = (value: number, noun: string) => `${value.toLocaleString()} ${noun}${value === 1 ? "" : "s"}`;

type Props = {
  node: TopologyNode;
  callers: FlowLink[];
  callees: FlowLink[];
  /** Null for a database or other dependency: only a service writes logs. */
  logs: UseQueryResult<Page<LogRecord>> | null;
  logLink: string;
  onSelect: (id: string) => void;
  onClose: () => void;
};

function Neighbours({ title, links, other, onSelect }: { title: string; links: FlowLink[]; other: "caller" | "callee"; onSelect: (id: string) => void }) {
  return <section className="drawer-section">
    <h3>{title}</h3>
    {links.length === 0 ? <p className="drawer-none">None observed</p> : <ul className="drawer-links">{links.map(({ edge, node }) => <li key={`${edge.source_node_id}>${edge.target_node_id}>${edge.kind}`}>
      <button type="button" aria-label={`${other === "caller" ? "Called by" : "Calls"} ${node.name}`} onClick={() => onSelect(node.id)}>
        <strong>{node.name}</strong>
        <small>{count(edge.call_count, "call")}{edge.error_count > 0 && <span className="drawer-errors"> · {count(edge.error_count, "error")}</span>} · avg {formatDuration(edge.average_latency_nanos)}</small>
      </button>
      {edge.trace_ids[0] && <DetailLink to={`/traces/${edge.trace_ids[0]}`}>Trace</DetailLink>}
    </li>)}</ul>}
  </section>;
}

/** What the graph's selected service did, shown beside the graph instead of in a second panel. */
export function ServiceDrawer({ node, callers, callees, logs, logLink, onSelect, onClose }: Props) {
  const total = (links: FlowLink[], field: "call_count" | "error_count") => links.reduce((sum, { edge }) => sum + edge[field], 0);
  const stats = [
    ["Calls in", total(callers, "call_count")], ["Calls out", total(callees, "call_count")],
    ["Errors on calls out", total(callees, "error_count")], ["Errors on calls in", total(callers, "error_count")],
  ] as const;
  return <aside className="topology-drawer" aria-label={`Details for ${node.name}`} onKeyDown={(event) => { if (event.key === "Escape") onClose(); }}>
    <header className="drawer-head">
      <div><h2>{node.name}</h2><span className="drawer-type">{node.type}</span></div>
      <button type="button" className="drawer-close" aria-label="Close detail" onClick={onClose}><X size={16} aria-hidden="true" /></button>
    </header>
    <dl className="drawer-stats">{stats.map(([label, value]) => <div key={label} className={label.startsWith("Errors") && value > 0 ? "bad" : ""}><dt>{label}</dt><dd>{value.toLocaleString()}</dd></div>)}</dl>
    <Neighbours title="Called by" links={callers} other="caller" onSelect={onSelect} />
    <Neighbours title="Calls out" links={callees} other="callee" onSelect={onSelect} />
    <section className="drawer-section">
      <div className="drawer-logs-head"><h3>Recent logs</h3>{logs && <Link to={logLink}>Open in Log explorer →</Link>}</div>
      {!logs ? <Empty>{node.type} dependencies do not write logs. Select a caller to read its logs.</Empty>
        : logs.isPending ? <Loading />
        : logs.isError ? <ErrorState error={logs.error} retry={() => void logs.refetch()} />
        : logs.data.data.length ? <ul className="drawer-log-list">{logs.data.data.slice(0, shownLogs).map((log) => <li key={log.event_id}>
          <time>{formatTime(log.occurred_at)}</time><Badge value={log.severity_text} />
          <code>{logBodyText(log.body)}</code>
          {log.trace_id && <DetailLink to={`/traces/${log.trace_id}`}>Trace</DetailLink>}
        </li>)}</ul> : <Empty>No logs from {node.name} in the selected time range.</Empty>}
    </section>
  </aside>;
}
