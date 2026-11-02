import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useParams } from "react-router-dom";
import { useAuth } from "../../app/auth";
import { api, pageItems, usePagedQuery } from "../../shared/api";
import type { Incident, IncidentEvent, IncidentOccurrence, StatusChange } from "../../shared/types";
import { BackLink, Badge, DetailLink, ErrorState, Loading, PageHeader, PagedTable, Panel, formatTime } from "../../shared/ui";
import { IncidentAnalysisPanels } from "./incident-analysis";

const transitions: Record<string, string[]> = {
  OPEN: ["INVESTIGATING", "RESOLVED", "IGNORED"],
  INVESTIGATING: ["RESOLVED", "IGNORED"],
  RESOLVED: ["OPEN"],
  IGNORED: ["OPEN"],
};

export function IncidentPage() {
  const { incidentID = "" } = useParams();
  const { credential } = useAuth();
  const queryClient = useQueryClient();
  const path = `/api/v1/incidents/${incidentID}`;
  const incident = useQuery({
    queryKey: ["incident", incidentID],
    queryFn: ({ signal }) => api<Incident>(path, credential, { signal }),
  });
  const occurrences = usePagedQuery<IncidentOccurrence>(["incident-occurrences", incidentID], `${path}/occurrences?limit=100`, credential);
  const events = usePagedQuery<IncidentEvent>(["incident-events", incidentID], `${path}/events?limit=100`, credential);
  const changes = usePagedQuery<StatusChange>(["incident-changes", incidentID], `${path}/status-changes?limit=100`, credential);
  const mutation = useMutation({
    mutationFn: (status: string) => api<Incident>(path, credential, { method: "PATCH", body: JSON.stringify({ status }) }),
    onSuccess: async () => {
      await Promise.all([
        ["incident", incidentID], ["incident-changes", incidentID], ["incidents"], ["overview"],
      ].map((queryKey) => queryClient.invalidateQueries({ queryKey })));
    },
  });
  if (incident.isPending) return <Loading page />;
  if (incident.isError) return <ErrorState error={incident.error} retry={() => void incident.refetch()} />;
  const record = incident.data;
  const occurrenceItems = pageItems(occurrences.data?.pages);
  const eventItems = pageItems(events.data?.pages);
  const changeItems = pageItems(changes.data?.pages);
  return <>
    <PageHeader title="Incident investigation" action={<BackLink fallback="/incidents" fallbackLabel="incidents" />}>
      {incidentID}
    </PageHeader>
    <Panel title={record.title} meta={`first seen ${formatTime(record.first_seen_at)}`}>
      <div className="incident-summary">
        <Fact label="Status"><Badge value={record.status} /></Fact>
        <Fact label="Severity"><Badge value={record.severity} /></Fact>
        <Fact label="Occurrences">{record.occurrence_count}</Fact>
        <Fact label="Service">{record.primary_service}</Fact>
        <Fact label="Last seen">{formatTime(record.last_seen_at)}</Fact>
        {record.error_message && <Fact label="Error">{record.error_message}</Fact>}
        {record.trace_id && <Fact label="Trace"><TraceLink traceID={record.trace_id} /></Fact>}
      </div>
      <div className="incident-actions">
        <span className="muted">Change status</span>
        {(transitions[record.status] || []).map((status) =>
          <button disabled={mutation.isPending} onClick={() => mutation.mutate(status)} key={status}>{status}</button>)}
        {mutation.isError && <span className="form-error" role="alert">{mutation.error.message}</span>}
      </div>
    </Panel>
    <IncidentAnalysisPanels incidentID={incidentID} />
    <Panel title="Occurrence timeline" meta={`${occurrenceItems.length} loaded`}>
      <PagedTable query={occurrences} headers={["Time", "Service", "Operation", "Error", "Trace"]}
        empty="No occurrences are retained for this incident."
        rows={occurrenceItems.map((item) => [formatTime(item.occurred_at), item.service_name, item.operation || "—",
          item.error_message || item.error_type || "—", <TraceLink traceID={item.trace_id} />])} />
    </Panel>
    <Panel title="Linked telemetry" meta={`${eventItems.length} loaded`}>
      <PagedTable query={events} headers={["Time", "Type", "Service", "Operation", "Error"]}
        empty="No telemetry events are linked to this incident."
        rows={eventItems.map((item) => [formatTime(item.occurred_at), item.event_type, item.service_name,
          item.operation || "—", item.error_message || item.error_type || "—"])} />
    </Panel>
    <Panel title="Status history" meta={`${changeItems.length} loaded`}>
      <PagedTable query={changes} headers={["Changed", "From", "To", "Actor"]}
        empty="No status changes have been recorded."
        rows={changeItems.map((item) => [formatTime(item.changed_at), <Badge value={item.previous_status} />,
          <Badge value={item.status} />, item.actor_user_id ? `user ${item.actor_user_id.slice(0, 8)}` : "system"])} />
    </Panel>
  </>;
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return <div className="incident-fact"><span>{label}</span><strong>{children}</strong></div>;
}

function TraceLink({ traceID }: { traceID?: string }) {
  return traceID ? <DetailLink to={`/traces/${traceID}`}>Open trace</DetailLink> : <>—</>;
}
