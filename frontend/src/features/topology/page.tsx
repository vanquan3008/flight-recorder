import { secondaryButtonClass } from "../../shared/ui/styles";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useAuth } from "../../shared/session";
import { api, queryString } from "../../shared/api";
import { FlowDiagram, GraphLegend, ServiceGraph, type FlowLink } from "../../shared/charts";
import type { LogRecord, Page, Topology, TopologyEdge, TopologyNode } from "../../shared/types";
import { Badge, DataTable, DetailLink, Empty, ErrorState, Loading, PageHeader, Panel, TimeCell, TimeWindowFilter, formatDuration, formatTime, useTimeWindow } from "../../shared/ui";
import { ServiceDrawer } from "./drawer";
import { FormattedLogLine, normalizeSeverity } from "./log-formatter";

const recentLogLimit = "20";
const count = (value: number, noun: string) => `${value} ${noun}${value === 1 ? "" : "s"}`;
const isService = (node: TopologyNode) => node.type.toUpperCase() === "SERVICE";

export function TopologyPage() {
  const { credential } = useAuth();
  const window = useTimeWindow();
  const [selected, setSelected] = useState("");
  // The view lives in the URL, so it survives a refresh and the browser Back button returns to the other one.
  const [params, setParams] = useSearchParams();
  const view = params.get("view") === "graph" ? "graph" : "list";
  const setView = (value: "list" | "graph") => {
    const next = new URLSearchParams(params);
    if (value === "graph") next.set("view", "graph"); else next.delete("view");
    setParams(next);
  };
  const path = `/api/v1/topology?${queryString({ from: window.from, to: window.to })}`;
  const query = useQuery({ queryKey: ["topology", path], queryFn: ({ signal }) => api<Topology>(path, credential, { signal }) });
  const nodes = query.data?.nodes ?? [];
  const edges = query.data?.edges ?? [];
  const callersOf = (id: string) => edges.filter((edge) => edge.target_node_id === id);
  const calleesOf = (id: string) => edges.filter((edge) => edge.source_node_id === id);
  const errorsOf = (id: string) => calleesOf(id).reduce((sum, edge) => sum + edge.error_count, 0);
  const services = [...nodes].sort((a, b) => errorsOf(b.id) - errorsOf(a.id) || a.name.localeCompare(b.name));
  // The detail is never empty: until the operator picks one, the service with the most errors is open.
  const node = nodes.find((item) => item.id === selected) ?? services[0];
  // The graph starts with nothing selected; only the list opens on a service.
  const active = view === "graph" ? nodes.find((item) => item.id === selected) : node;
  // Only a service writes logs, so a database or external dependency has nothing to list.
  const logsPath = active && isService(active) ? `/api/v1/logs?${queryString({ service: active.name, from: window.from, to: window.to, limit: recentLogLimit })}` : "";
  const logs = useQuery({ queryKey: ["topology-logs", logsPath], enabled: Boolean(logsPath), queryFn: ({ signal }) => api<Page<LogRecord>>(logsPath, credential, { signal }) });
  if (query.isPending) return <Loading page />;
  if (query.isError) return <ErrorState error={query.error} retry={() => void query.refetch()} />;
  const names = new Map(nodes.map((item) => [item.id, item.name]));
  const label = (id: string) => names.get(id) ?? id;
  const edgeRow = (edge: TopologyEdge) => [label(edge.source_node_id), label(edge.target_node_id), edge.kind, edge.call_count, edge.error_count, formatDuration(edge.average_latency_nanos), formatDuration(edge.p95_latency_nanos),
    edge.trace_ids[0] ? <DetailLink to={`/traces/${edge.trace_ids[0]}`}>Open trace</DetailLink> : "—"];
  const headers = ["From", "To", "Kind", "Calls", "Errors", "Avg", "P95", "Evidence"];
  const byId = new Map(nodes.map((item) => [item.id, item]));
  const link = (edge: TopologyEdge, otherId: string): FlowLink => ({ edge, node: byId.get(otherId) ?? { id: otherId, type: "UNKNOWN", name: otherId } });
  const toggle = <div className="segmented inline-flex min-w-0 gap-0.5 rounded-lg border border-glass-field-edge bg-glass-field p-0.75 view-switch [&>button]:min-w-16 [&>button]:rounded-md [&>button]:px-4 [&>button]:py-1.25 [&>button]:text-xs [&>button]:font-semibold [&>button]:text-muted [&>button:hover]:text-text [&>button[aria-pressed=true]]:bg-btn [&>button[aria-pressed=true]]:text-btn-text" role="group" aria-label="Topology view">
    {(["list", "graph"] as const).map((value) => <button type="button" key={value} aria-pressed={view === value} onClick={() => setView(value)}>{value === "list" ? "List" : "Graph"}</button>)}
  </div>;
  const logLink = active ? `/logs?${queryString({ service: active.name, from: window.from, to: window.to })}` : "";
  const detailBody = node && <>
    <div className="flow-canvas relative m-3 overflow-hidden rounded-2xl border border-glass-divider bg-glass-sunken bg-[radial-gradient(circle_at_1px_1px,color-mix(in_srgb,var(--text)_14%,transparent)_1px,transparent_0)] bg-size-[24px_24px] p-5 shadow-inner">
      <div className="flow-canvas-hud mb-3 flex items-center justify-between text-[11px] font-semibold text-muted">
        <span className="inline-flex items-center gap-2 rounded-full border border-glass-divider bg-surface-solid px-3 py-1 shadow-xs">
          <i className="size-2 rounded-full bg-accent shadow-[0_0_6px_var(--accent)]" />
          <span className="font-mono text-[10px] font-bold tracking-wider text-text uppercase">Telemetry Flow Architecture</span>
        </span>
        <span className="font-mono text-[11px] text-faint">
          {callersOf(node.id).length + calleesOf(node.id).length} active connections
        </span>
      </div>
      <div className="flow-position overflow-x-auto overflow-y-visible" role="group" aria-label={`Where ${node.name} sits in the flow`}>
        <FlowDiagram node={node} onSelect={setSelected}
          callers={callersOf(node.id).map((edge) => link(edge, edge.source_node_id))}
          callees={calleesOf(node.id).map((edge) => link(edge, edge.target_node_id))} />
      </div>
    </div>
    <div className="topology-logs flex flex-col border-t border-glass-divider px-4.5 pt-3 pb-3.5 [&_h3]:m-0 [&_h3]:text-[10.5px] [&_h3]:font-bold [&_h3]:tracking-wider [&_h3]:text-faint [&_h3]:uppercase">
      <div className="topology-logs-head mb-2 flex items-center justify-between [&>a]:text-[11.5px] [&>a]:font-semibold [&>a]:text-accent [&>a]:no-underline [&>a:hover]:underline"><h3>Recent logs</h3>{isService(node) && <Link to={logLink}>Open in Log explorer →</Link>}</div>
      {!isService(node) ? <Empty>{node.type} dependencies do not write logs. Open a caller to read its logs.</Empty>
        : logs.isPending ? <Loading />
        : logs.isError ? <ErrorState error={logs.error} retry={() => void logs.refetch()} />
        : logs.data.data.length ? <ul className="topology-log-list m-0 flex max-h-[300px] min-h-0 list-none flex-col gap-0.75 overflow-x-hidden overflow-y-auto rounded-xl border border-glass-divider bg-glass-nested p-1.5 [&>li]:grid [&>li]:grid-cols-[145px_80px_minmax(0,1fr)_auto] [&>li]:items-center [&>li]:gap-3 [&>li]:rounded-lg [&>li]:px-3 [&>li]:py-1.75 [&>li]:text-xs [&>li:hover]:bg-glass-hover [&_time]:flex [&_time]:min-w-0 [&_time]:overflow-hidden [&_time]:font-mono [&_time]:text-[11px] [&_time]:text-muted [&_.badge]:justify-center [&_.badge]:text-[10px] max-[760px]:[&>li]:grid-cols-[minmax(0,1fr)_auto] max-[760px]:[&>li]:gap-1.5 max-[760px]:[&_.log-line-code]:col-span-full max-[760px]:[&_.log-trace-pill]:col-start-2 max-[760px]:[&_.log-trace-pill]:justify-self-end">{logs.data.data.map((log) => <li key={log.event_id}>
          <time title={formatTime(log.occurred_at)}><TimeCell value={log.occurred_at} /></time>
          <Badge value={normalizeSeverity(log.severity_text)} />
          <FormattedLogLine body={log.body} />
          {log.trace_id ? <DetailLink to={`/traces/${log.trace_id}`} className={`log-trace-pill ${secondaryButtonClass} rounded-full px-2.25 py-0.5 text-[11px] text-accent no-underline`}>Trace ↗</DetailLink> : <span />}
        </li>)}</ul> : <Empty>No logs from {node.name} in the selected time range.</Empty>}
    </div>
    <details className="topology-calls border-t border-glass-divider [&>summary]:cursor-pointer [&>summary]:px-4.5 [&>summary]:py-2.5 [&>summary]:text-xs [&>summary]:font-semibold [&>summary]:text-muted [&_h3]:px-4.5 [&_h3]:pt-1.5 [&_h3]:text-[10.5px] [&_h3]:text-faint [&_h3]:uppercase">
      <summary>Call details ({callersOf(node.id).length + calleesOf(node.id).length})</summary>
      <h3>Calls out</h3>
      <DataTable headers={headers} empty="This node makes no observed calls." rows={calleesOf(node.id).map(edgeRow)} />
      <h3>Called by</h3>
      <DataTable headers={headers} empty="No observed callers." rows={callersOf(node.id).map(edgeRow)} />
    </details>
  </>;
  return (
    <div className="topology-page flex h-full max-h-full min-h-0 w-full max-w-full flex-1 flex-col overflow-hidden pb-1">
      <div className="shrink-0">
        <PageHeader title="Service and dependency topology" action={<div className="header-controls flex flex-wrap items-center justify-end gap-3">{toggle}<TimeWindowFilter {...window} /></div>}>
          Derived from parent/child spans and OpenTelemetry database, messaging, and client semantic attributes.
        </PageHeader>
      </div>
      {!node ? (
        <Panel title="Services"><Empty>No services were observed in the selected time range.</Empty></Panel>
      ) : view === "graph" ? (
        <section className="panel flex flex-1 min-h-0 flex-col overflow-hidden rounded-3xl border border-glass-edge bg-glass backdrop-glass shadow-glass topology-graph-panel mb-2" aria-label="Dependency graph">
          <header className="topology-bar shrink-0 flex min-h-12 items-center justify-between gap-2.5 border-b border-glass-divider px-5 py-2.5 [&>h2]:m-0 [&>h2]:text-[14px] [&>h2]:font-bold">
            <div className="flex items-center gap-2.5">
              <i className="size-2.5 rounded-full bg-accent shadow-[0_0_8px_var(--accent)]" />
              <h2>Dependency graph</h2>
              <span className="inline-flex items-center rounded-full border border-glass-divider bg-glass-field px-2.5 py-0.5 font-mono text-[10px] font-semibold text-muted">
                {nodes.length} nodes · {edges.length} relationships
              </span>
            </div>
            <span className="topology-count font-mono text-[11px] text-faint">Interactive DAG View</span>
          </header>
          <div className={`topology-canvas flex-1 min-h-0 grid ${active ? "grid-cols-[minmax(0,1fr)_390px] with-drawer" : "grid-cols-1"} max-[900px]:grid-cols-1 overflow-hidden`}>
            <ServiceGraph nodes={nodes} edges={edges} selected={active?.id ?? ""} onSelect={setSelected} label="Service topology graph" />
            {active && <ServiceDrawer node={active} logLink={logLink} logs={isService(active) ? logs : null} onSelect={setSelected} onClose={() => setSelected("")}
              callers={callersOf(active.id).map((edge) => link(edge, edge.source_node_id))} callees={calleesOf(active.id).map((edge) => link(edge, edge.target_node_id))} />}
          </div>
          <div className="shrink-0">
            <GraphLegend nodes={nodes} />
          </div>
        </section>
      ) : (
        <section className="panel flex flex-1 min-h-0 overflow-hidden rounded-3xl border border-glass-edge bg-glass backdrop-glass shadow-glass topology-workspace grid grid-cols-[300px_minmax(0,1fr)] max-[900px]:min-h-0 max-[900px]:grid-cols-1 mb-2" aria-label="Services and detail">
          <aside className="topology-aside flex min-h-0 min-w-0 flex-col border-r border-glass-divider max-[900px]:border-r-0 max-[900px]:border-b">
            <header className="topology-bar shrink-0 flex min-h-11 items-center justify-between gap-2.5 border-b border-glass-divider px-4.5 py-2 [&>h2]:m-0 [&>h2]:text-[13px] [&>h2]:font-semibold">
              <h2>Services</h2>
              <span className="topology-count font-mono text-[11.5px] font-semibold text-faint">{services.length}</span>
            </header>
            <ul className="service-list m-0 flex-1 min-h-0 grid list-none content-start gap-0.5 overflow-y-auto p-1.5">{services.map((item) => {
              const errors = errorsOf(item.id);
              return <li key={item.id}><button type="button" className="service-row grid w-full grid-cols-[1fr_auto] gap-x-2 gap-y-px rounded-xl px-3 py-2 text-left hover:bg-glass-hover aria-pressed:bg-accent/16" aria-pressed={node.id === item.id} onClick={() => setSelected(item.id)}>
                <span className="service-name text-[13px] font-semibold wrap-anywhere">{item.name}</span>{" "}
                <span className="service-type self-center text-[10px] font-bold tracking-wider text-faint uppercase">{item.type}</span>{" "}
                <span className="service-meta col-span-full text-[11.5px] text-muted">{callersOf(item.id).length} in · {calleesOf(item.id).length} out{errors > 0 && <span className="service-errors text-danger-fg"> · {count(errors, "error")}</span>}</span>
              </button></li>;
            })}</ul>
          </aside>
          <div className="topology-main flex min-h-0 min-w-0 flex-1 flex-col overflow-y-auto">
            <header className="topology-bar shrink-0 flex min-h-11 items-center justify-between gap-2.5 border-b border-glass-divider px-4.5 py-2 [&>h2]:m-0 [&>h2]:text-[13.5px] [&>h2]:font-semibold">
              <div className="flex items-center gap-2.5">
                <i className="size-2 rounded-full bg-accent shadow-[0_0_8px_var(--accent)]" />
                <h2>{node.name}</h2>
                <span className="service-type inline-flex items-center rounded-full border border-glass-divider bg-glass-field px-2 py-0.5 font-mono text-[9.5px] font-bold tracking-wider text-faint uppercase">{node.type}</span>
              </div>
              <div className="flex items-center gap-2">
                {errorsOf(node.id) > 0 ? (
                  <span className="inline-flex items-center gap-1 rounded-full border border-red-500/30 bg-red-500/10 px-2 py-0.5 font-mono text-[10px] font-semibold text-danger-fg">
                    <i className="size-1.5 rounded-full bg-current animate-pulse" />
                    {count(errorsOf(node.id), "error")}
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1 rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2 py-0.5 font-mono text-[10px] font-semibold text-emerald-500">
                    <i className="size-1.5 rounded-full bg-current" />
                    Nominal
                  </span>
                )}
              </div>
            </header>
            {detailBody}
          </div>
        </section>
      )}
    </div>
  );
}
