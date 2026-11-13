import { secondaryButtonClass } from "../../shared/ui/styles";
import { X } from "lucide-react";
import { Link } from "react-router-dom";
import type { UseQueryResult } from "@tanstack/react-query";
import type { FlowLink } from "../../shared/charts";
import type { LogRecord, Page, TopologyNode } from "../../shared/types";
import { Badge, DetailLink, Empty, ErrorState, Loading, TimeCell, formatCount, formatDuration, formatTime } from "../../shared/ui";
import { FormattedLogLine, normalizeSeverity } from "./log-formatter";

const shownLogs = 8;

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
  return <section className="drawer-section border-t border-glass-divider px-4.5 py-3 [&_h3]:mb-0 [&_h3]:text-[10px] [&_h3]:font-bold [&_h3]:tracking-widest [&_h3]:text-faint [&_h3]:uppercase">
    <div className="mb-2 flex items-center justify-between">
      <h3>{title}</h3>
      <span className="font-mono text-[10px] font-semibold text-faint">{links.length}</span>
    </div>
    {links.length === 0 ? (
      <div className="drawer-none rounded-lg border border-dashed border-glass-divider p-2 text-center font-mono text-[11px] text-faint">
        None observed
      </div>
    ) : (
      <ul className="drawer-links m-0 grid list-none gap-1.5 p-0 [&>li]:flex [&>li]:items-center [&>li]:gap-2">
        {links.map(({ edge, node }) => {
          const failing = edge.error_count > 0;
          return (
            <li key={`${edge.source_node_id}>${edge.target_node_id}>${edge.kind}`}>
              <button type="button" className="group grid min-w-0 flex-1 rounded-xl border border-glass-field-edge bg-glass-field px-3 py-2 text-left transition-all hover:border-accent hover:bg-glass-field-focus" aria-label={`${other === "caller" ? "Called by" : "Calls"} ${node.name}`} onClick={() => onSelect(node.id)}>
                <div className="flex items-center gap-1.5">
                  <i className={`size-1.5 rounded-full ${failing ? "bg-danger-fg animate-pulse" : "bg-emerald-500"}`} />
                  <strong className="truncate text-[12.5px] font-semibold text-text group-hover:text-accent">{node.name}</strong>
                </div>
                <small className="mt-0.5 font-mono text-[10.5px] text-muted">
                  {formatCount(edge.call_count, "call")}
                  {edge.error_count > 0 && <span className="drawer-errors font-bold text-danger-fg"> · {formatCount(edge.error_count, "error")}</span>}
                  {" · avg "}{formatDuration(edge.average_latency_nanos)}
                </small>
              </button>
              {edge.trace_ids[0] && (
                <DetailLink to={`/traces/${edge.trace_ids[0]}`} className={`log-trace-pill ${secondaryButtonClass} rounded-full px-2.25 py-1 text-[11px] text-accent no-underline`}>
                  Trace
                </DetailLink>
              )}
            </li>
          );
        })}
      </ul>
    )}
  </section>;
}

/** What the graph's selected service did, shown beside the graph instead of in a second panel. */
export function ServiceDrawer({ node, callers, callees, logs, logLink, onSelect, onClose }: Props) {
  const total = (links: FlowLink[], field: "call_count" | "error_count") => links.reduce((sum, { edge }) => sum + edge[field], 0);
  const stats = [
    ["Calls in", total(callers, "call_count")], ["Calls out", total(callees, "call_count")],
    ["Errors on calls out", total(callees, "error_count")], ["Errors on calls in", total(callers, "error_count")],
  ] as const;
  return <aside className="topology-drawer flex h-full max-h-none min-h-0 min-w-0 flex-col overflow-y-auto border-l border-glass-divider bg-glass-nested max-[900px]:border-t max-[900px]:border-l-0" aria-label={`Details for ${node.name}`} onKeyDown={(event) => { if (event.key === "Escape") onClose(); }}>
    <header className="drawer-head flex items-start justify-between gap-2.5 border-b border-glass-divider px-4.5 pt-3.5 pb-2.5">
      <div className="flex min-w-0 flex-col gap-1">
        <div className="flex items-center gap-2">
          <i className="size-2 rounded-full bg-accent shadow-[0_0_6px_var(--accent)]" />
          <h2 className="m-0 text-[15px] font-semibold tracking-tight wrap-anywhere">{node.name}</h2>
        </div>
        <div>
          <span className="drawer-type inline-flex items-center rounded-full border border-glass-divider bg-glass-field px-2 py-0.5 font-mono text-[9px] font-bold tracking-wider text-faint uppercase">{node.type}</span>
        </div>
      </div>
      <button type="button" className="drawer-close grid size-7 shrink-0 place-items-center rounded-lg border border-glass-divider bg-glass-field text-muted transition-colors hover:bg-glass-hover hover:text-text" aria-label="Close detail" onClick={onClose}><X size={15} aria-hidden="true" /></button>
    </header>
    <dl className="drawer-stats m-0 grid grid-cols-2 gap-2 px-4.5 py-3">
      {stats.map(([label, value]) => {
        const isError = label.startsWith("Errors");
        const bad = isError && value > 0;
        return (
          <div key={label} className={`rounded-xl border p-2.5 transition-colors ${bad ? "bad border-red-500/35 bg-red-500/10 text-danger-fg" : "border-glass-divider bg-glass-field"}`}>
            <dt className="flex items-center justify-between text-[9.5px] font-semibold tracking-wider text-faint uppercase">
              <span>{label}</span>
              {bad && <i className="size-1.5 rounded-full bg-current animate-pulse" />}
            </dt>
            <dd className={`mt-1 font-mono text-[17px] font-bold tabular-nums ${bad ? "text-danger-fg" : "text-text"}`}>
              {value.toLocaleString()}
            </dd>
          </div>
        );
      })}
    </dl>
    <Neighbours title="Called by" links={callers} other="caller" onSelect={onSelect} />
    <Neighbours title="Calls out" links={callees} other="callee" onSelect={onSelect} />
    <section className="drawer-section border-t border-glass-divider px-4.5 py-3 [&_h3]:mb-2 [&_h3]:text-[10px] [&_h3]:font-bold [&_h3]:tracking-widest [&_h3]:text-faint [&_h3]:uppercase">
      <div className="drawer-logs-head mb-2 flex items-baseline justify-between gap-2 [&>a]:text-[11.5px] [&>a]:font-semibold [&>a]:text-accent [&>a]:no-underline [&>a:hover]:underline"><h3>Recent logs</h3>{logs && <Link to={logLink}>Open in Log explorer →</Link>}</div>
      {!logs ? <Empty>{node.type} dependencies do not write logs. Select a caller to read its logs.</Empty>
        : logs.isPending ? <Loading />
        : logs.isError ? <ErrorState error={logs.error} retry={() => void logs.refetch()} />
        : logs.data.data.length ? <ul className="drawer-log-list m-0 flex list-none flex-col gap-1 p-0 [&>li]:grid [&>li]:grid-cols-[135px_76px_1fr] [&>li]:items-center [&>li]:gap-x-2.5 [&>li]:gap-y-1 [&>li]:rounded-lg [&>li]:border [&>li]:border-glass-divider [&>li]:bg-glass-field [&>li]:px-2.5 [&>li]:py-1.75 [&>li]:text-[11.5px] [&>li:hover]:bg-glass-hover [&_time]:flex [&_time]:min-w-0 [&_time]:overflow-hidden [&_time]:font-mono [&_time]:text-[10.5px] [&_time]:text-muted [&_.badge]:justify-center [&_.badge]:text-[9.5px] [&_.log-line-code]:col-span-full [&_.log-line-code]:text-[11px] [&_.log-trace-pill]:justify-self-end">{logs.data.data.slice(0, shownLogs).map((log) => <li key={log.event_id}>
          <time title={formatTime(log.occurred_at)}><TimeCell value={log.occurred_at} /></time>
          <Badge value={normalizeSeverity(log.severity_text)} />
          {log.trace_id && <DetailLink to={`/traces/${log.trace_id}`} className={`log-trace-pill ${secondaryButtonClass} rounded-full px-2.25 py-0.5 text-[11px] text-accent no-underline`}>Trace ↗</DetailLink>}
          <FormattedLogLine body={log.body} />
        </li>)}</ul> : <Empty>No logs from {node.name} in the selected time range.</Empty>}
    </section>
  </aside>;
}
