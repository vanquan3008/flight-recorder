import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useAuth } from "../../app/auth";
import { api, queryString } from "../../shared/api";
import { FlowDiagram, GraphLegend, ServiceGraph, type FlowLink } from "../../shared/charts";
import type { LogRecord, Page, Topology, TopologyEdge, TopologyNode } from "../../shared/types";
import { Badge, DataTable, DetailLink, Empty, ErrorState, Loading, PageHeader, Panel, TimeWindowFilter, formatDuration, formatTime, useTimeWindow } from "../../shared/ui";
import { ServiceDrawer } from "./drawer";
import { FormattedLogLine, normalizeSeverity } from "./log-formatter";
import "./topology.css";

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
  const toggle = <div className="segmented view-switch" role="group" aria-label="Topology view">
    {(["list", "graph"] as const).map((value) => <button type="button" key={value} aria-pressed={view === value} onClick={() => setView(value)}>{value === "list" ? "List" : "Graph"}</button>)}
  </div>;
  const logLink = active ? `/logs?${queryString({ service: active.name, from: window.from, to: window.to })}` : "";
  const detailBody = node && <>
    <div className="flow-position" role="group" aria-label={`Where ${node.name} sits in the flow`}>
      <FlowDiagram node={node} onSelect={setSelected}
        callers={callersOf(node.id).map((edge) => link(edge, edge.source_node_id))}
        callees={calleesOf(node.id).map((edge) => link(edge, edge.target_node_id))} />
    </div>
    <div className="topology-logs">
      <div className="topology-logs-head"><h3>Recent logs</h3>{isService(node) && <Link to={logLink}>Open in Log explorer →</Link>}</div>
      {!isService(node) ? <Empty>{node.type} dependencies do not write logs. Open a caller to read its logs.</Empty>
        : logs.isPending ? <Loading />
        : logs.isError ? <ErrorState error={logs.error} retry={() => void logs.refetch()} />
        : logs.data.data.length ? <ul className="topology-log-list">{logs.data.data.map((log) => <li key={log.event_id}>
          <time>{formatTime(log.occurred_at)}</time>
          <Badge value={normalizeSeverity(log.severity_text)} />
          <FormattedLogLine body={log.body} />
          {log.trace_id ? <DetailLink to={`/traces/${log.trace_id}`} className="log-trace-pill">Trace ↗</DetailLink> : <span />}
        </li>)}</ul> : <Empty>No logs from {node.name} in the selected time range.</Empty>}
    </div>
    <details className="topology-calls">
      <summary>Call details ({callersOf(node.id).length + calleesOf(node.id).length})</summary>
      <h3>Calls out</h3>
      <DataTable headers={headers} empty="This node makes no observed calls." rows={calleesOf(node.id).map(edgeRow)} />
      <h3>Called by</h3>
      <DataTable headers={headers} empty="No observed callers." rows={callersOf(node.id).map(edgeRow)} />
    </details>
  </>;
  return <>
    <PageHeader title="Service and dependency topology" action={<div className="header-controls">{toggle}<TimeWindowFilter range={window.range} select={window.select} /></div>}>Derived from parent/child spans and OpenTelemetry database, messaging, and client semantic attributes.</PageHeader>
    {!node ? <Panel title="Services"><Empty>No services were observed in the selected time range.</Empty></Panel>
      : view === "graph" ? <section className="panel topology-graph-panel" aria-label="Dependency graph">
        <header className="topology-bar"><h2>Dependency graph</h2><span className="topology-count">{nodes.length} nodes · {edges.length} relationships</span></header>
        <div className={`topology-canvas${active ? " with-drawer" : ""}`}>
          <ServiceGraph nodes={nodes} edges={edges} selected={active?.id ?? ""} onSelect={setSelected} label="Service topology graph" />
          {active && <ServiceDrawer node={active} logLink={logLink} logs={isService(active) ? logs : null} onSelect={setSelected} onClose={() => setSelected("")}
            callers={callersOf(active.id).map((edge) => link(edge, edge.source_node_id))} callees={calleesOf(active.id).map((edge) => link(edge, edge.target_node_id))} />}
        </div>
        <GraphLegend nodes={nodes} />
      </section> : <section className="panel topology-workspace" aria-label="Services and detail">
        <aside className="topology-aside">
          <header className="topology-bar"><h2>Services</h2><span className="topology-count">{services.length}</span></header>
          <ul className="service-list">{services.map((item) => {
            const errors = errorsOf(item.id);
            return <li key={item.id}><button type="button" className="service-row" aria-pressed={node.id === item.id} onClick={() => setSelected(item.id)}>
              <span className="service-name">{item.name}</span>{" "}
              <span className="service-type">{item.type}</span>{" "}
              <span className="service-meta">{callersOf(item.id).length} in · {calleesOf(item.id).length} out{errors > 0 && <span className="service-errors"> · {count(errors, "error")}</span>}</span>
            </button></li>;
          })}</ul>
        </aside>
        <div className="topology-main">
          <header className="topology-bar"><h2>{node.name}</h2><span className="service-type">{node.type}</span></header>
          {detailBody}
        </div>
      </section>}
  </>;
}
