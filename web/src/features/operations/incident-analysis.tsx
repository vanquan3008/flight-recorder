import { useQuery } from "@tanstack/react-query";
import { useAuth } from "../../app/auth";
import { api } from "../../shared/api";
import { DetailLink } from "../../shared/navigation";
import type { IncidentAnalysis } from "../../shared/types";
import { DataTable, ErrorState, Loading, Panel } from "../../shared/ui";

export function IncidentAnalysisPanels({ incidentID }: { incidentID: string }) {
  const { apiKey } = useAuth();
  const query = useQuery({
    queryKey: ["incident-analysis", incidentID],
    queryFn: ({ signal }) => api<IncidentAnalysis>(`/api/v1/incidents/${incidentID}/analysis`, apiKey, { signal }),
  });
  if (query.isPending) return <Panel title="Incident analysis"><Loading /></Panel>;
  if (query.isError) return <Panel title="Incident analysis"><ErrorState error={query.error} retry={() => void query.refetch()} /></Panel>;
  const analysis = query.data;
  return <>
    <Panel title="RCA hypotheses" meta="heuristic ranking · not proof">
      <p>{analysis.conclusion}</p>
      <DataTable headers={["Rank", "Entity", "Hypothesis", "Heuristic score", "Evidence IDs"]}
        empty="Not enough evidence has been captured yet."
        rows={analysis.hypotheses.map((item) => [item.rank, item.entity_id, item.statement,
          item.confidence.toFixed(2), item.evidence_ids.join(", ")])} />
    </Panel>
    <Panel title="Evidence relationships" meta={`${analysis.causal_graph.length} supported correlations`}>
      <DataTable headers={["Evidence A", "Evidence B", "Relationship", "Heuristic score"]}
        empty="A dependency, a shared trace, and overlapping evidence windows are required."
        rows={analysis.causal_graph.map((edge) => [edge.source_entity_id, edge.target_entity_id,
          edge.relationship, edge.confidence.toFixed(2)])} />
    </Panel>
    <Panel title="Evidence" meta={`${analysis.evidence.length} records`}>
      <DataTable headers={["Signal", "Entity", "Metric", "Series", "Direction", "Observed", "Baseline", "Trace"]}
        empty="No evidence is stored for this incident."
        rows={analysis.evidence.map((item) => [item.signal_type, item.entity_id, item.metric_name, item.series_id || "—",
          item.direction, item.observed_value.toPrecision(4), item.baseline_value.toPrecision(4),
          item.trace_ids[0] ? <DetailLink to={`/traces/${item.trace_ids[0]}`}>Open trace</DetailLink> : "—"])} />
    </Panel>
  </>;
}
