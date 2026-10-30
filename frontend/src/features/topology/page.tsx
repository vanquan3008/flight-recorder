import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useAuth } from "../../app/auth";
import { api, queryString } from "../../shared/api";
import { DetailLink } from "../../shared/navigation";
import { TimeWindowFilter, useTimeWindow } from "../../shared/time-window";
import type { Topology, TopologyEdge } from "../../shared/types";
import { DataTable, Empty, ErrorState, Loading, PageHeader, Panel, formatDuration } from "../../shared/ui";
import { ServiceGraph } from "../../shared/service-graph";
import "./topology.css";

export function TopologyPage() {
  const { apiKey } = useAuth();
  const window = useTimeWindow();
  const [selected, setSelected] = useState("");
  const path = `/api/v1/topology?${queryString({ from: window.from, to: window.to })}`;
  const query = useQuery({ queryKey: ["topology", path], queryFn: ({ signal }) => api<Topology>(path, apiKey, { signal }) });
  if (query.isPending) return <Loading />;
  if (query.isError) return <ErrorState error={query.error} retry={() => void query.refetch()} />;
  const topology = query.data;
  const names = new Map(topology.nodes.map((node) => [node.id, node.name]));
  const label = (id: string) => names.get(id) ?? id;
  const node = topology.nodes.find((item) => item.id === selected);
  const edgeRow = (edge: TopologyEdge) => [label(edge.source_node_id), label(edge.target_node_id), edge.kind, edge.call_count, edge.error_count, formatDuration(edge.average_latency_nanos), formatDuration(edge.p95_latency_nanos),
    edge.trace_ids[0] ? <DetailLink to={`/traces/${edge.trace_ids[0]}`}>Open trace</DetailLink> : "—"];
  const headers = ["From", "To", "Kind", "Calls", "Errors", "Avg", "P95", "Evidence"];
  return <>
    <PageHeader title="Service and dependency topology">Derived from parent/child spans and OpenTelemetry database, messaging, and client semantic attributes.</PageHeader>
    <TimeWindowFilter range={window.range} select={window.select} />
    <Panel title="Dependency graph" meta={`${topology.nodes.length} nodes · ${topology.edges.length} relationships`}>
      {topology.nodes.length ? <>
        <ServiceGraph nodes={topology.nodes} edges={topology.edges} selected={selected} onSelect={setSelected} label="Service topology graph" />
        <p className="graph-legend muted">Arrows point from caller to callee. Line width follows call count; dashed red lines have errors. Select a node to see its calls.</p>
      </> : <Empty>No services were observed in the selected time range.</Empty>}
    </Panel>
    {node && <Panel title={node.name} meta={`${node.type} · ${node.id}`}>
      <div className="topology-detail">
        <h3>Calls out</h3>
        <DataTable headers={headers} empty="This node makes no observed calls." rows={topology.edges.filter((edge) => edge.source_node_id === node.id).map(edgeRow)} />
        <h3>Called by</h3>
        <DataTable headers={headers} empty="No observed callers." rows={topology.edges.filter((edge) => edge.target_node_id === node.id).map(edgeRow)} />
      </div>
    </Panel>}
    <Panel title="Observed edges" meta={`${topology.edges.length} relationships`}><DataTable headers={headers} empty="No service or dependency edges were found." rows={topology.edges.map(edgeRow)} /></Panel>
  </>;
}
