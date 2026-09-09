import { useMutation, useQueries } from "@tanstack/react-query";
import { ArrowLeft, Search } from "lucide-react";
import type { FormEvent, ReactNode } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { api, pageItems, queryString, usePagedQuery } from "../api";
import { useAuth } from "../auth";
import type { Incident, IncidentEvent, IncidentOccurrence, LogRecord, Page, StatusChange } from "../types";
import { Badge, DataTable, ErrorState, LoadMore, Loading, PageHeader, Panel, formatTime, json } from "../ui";
import { useQueryClient } from "@tanstack/react-query";

export function MachineLogsPage() {
  const { apiKey } = useAuth();
  const [params, setParams] = useSearchParams();
  const path = `/api/v1/logs?${queryString({ service: params.get("service") || "file-agent" })}`;
  const query = usePagedQuery<LogRecord>(["machine-logs"], path, apiKey);
  const logs = pageItems(query.data?.pages);
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setParams(new URLSearchParams([...form.entries()].filter(([, value]) => String(value).trim()).map(([key, value]) => [key, String(value)])));
  };
  return <><PageHeader title="Machine logs">Logs shipped by tracepilot-agent, with host and source file context.</PageHeader><form className="toolbar" onSubmit={submit}><input name="service" placeholder="Agent service" aria-label="Filter by agent service" defaultValue={params.get("service") || "file-agent"} /><button><Search size={16} /> Load logs</button></form><Panel title="Machine log records" meta={`${logs.length} loaded`}><PagedTable query={query} empty="No machine logs match this service." headers={["Time", "Host", "File", "Severity", "Message"]} rows={logs.map((log) => {
    const attrs = object(log.resource_attributes);
    return [formatTime(log.occurred_at), attrs["host.name"] || "unknown", attrs["log.file.path"] || "unknown", <Badge value={log.severity_text} />, <pre className="inline-json">{json(log.body)}</pre>];
  })} /></Panel></>;
}

export function OperationsPage() {
  const { apiKey } = useAuth();
  const [params, setParams] = useSearchParams();
  const path = `/api/v1/incidents?${queryString({ service: params.get("service") || undefined, status: params.get("status") || undefined })}`;
  const query = usePagedQuery<Incident>(["incidents"], path, apiKey);
  const incidents = pageItems(query.data?.pages);
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setParams(new URLSearchParams([...form.entries()].filter(([, value]) => String(value).trim()).map(([key, value]) => [key, String(value)])));
  };
  return <><PageHeader title="Incidents">Investigate application failures from their timeline and correlated telemetry.</PageHeader><form className="toolbar" onSubmit={submit}><input name="service" placeholder="Service" aria-label="Filter by service" defaultValue={params.get("service") || ""} /><select name="status" aria-label="Filter by status" defaultValue={params.get("status") || ""}><option value="">All statuses</option><option>OPEN</option><option>INVESTIGATING</option><option>RESOLVED</option><option>IGNORED</option></select><button><Search size={16} /> Apply filters</button></form><Panel title="Incident timeline" meta={`${incidents.length} loaded`}><PagedTable query={query} empty="No incidents yet." headers={["Last seen", "Severity", "Status", "Title", "Occurrences", ""]} rows={incidents.map((incident) => [
    formatTime(incident.last_seen_at), <Badge value={incident.severity} />, <Badge value={incident.status} />, incident.title, incident.occurrence_count, <Link className="table-link" to={`/incidents/${incident.id}`}>Inspect</Link>,
  ])} /></Panel></>;
}

export function IncidentPage() {
  const { incidentID = "" } = useParams();
  const { apiKey } = useAuth();
  const queryClient = useQueryClient();
  const [incident, occurrences, events, changes] = useQueries({ queries: [
    { queryKey: ["incident", incidentID], queryFn: ({ signal }) => api<Incident>(`/api/v1/incidents/${incidentID}`, apiKey, { signal }) },
    { queryKey: ["incident-occurrences", incidentID], queryFn: ({ signal }) => api<Page<IncidentOccurrence>>(`/api/v1/incidents/${incidentID}/occurrences?limit=100`, apiKey, { signal }) },
    { queryKey: ["incident-events", incidentID], queryFn: ({ signal }) => api<Page<IncidentEvent>>(`/api/v1/incidents/${incidentID}/events?limit=100`, apiKey, { signal }) },
    { queryKey: ["incident-changes", incidentID], queryFn: ({ signal }) => api<Page<StatusChange>>(`/api/v1/incidents/${incidentID}/status-changes?limit=100`, apiKey, { signal }) },
  ] });
  const mutation = useMutation({ mutationFn: (status: string) => api<Incident>(`/api/v1/incidents/${incidentID}`, apiKey, { method: "PATCH", body: JSON.stringify({ status }) }), onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["incident", incidentID] }).then(() => queryClient.invalidateQueries({ queryKey: ["incidents"] })) });
  const queries = [incident, occurrences, events, changes];
  if (queries.some((query) => query.isPending)) return <Loading />;
  const failed = queries.find((query) => query.isError);
  if (failed) return <ErrorState error={failed.error} retry={() => queries.forEach((query) => void query.refetch())} />;
  const record = incident.data!;
  const occurrenceItems = occurrences.data!.data;
  const eventItems = events.data!.data;
  const changeItems = changes.data!.data;
  const transitions: Record<string, string[]> = { OPEN: ["INVESTIGATING", "RESOLVED", "IGNORED"], INVESTIGATING: ["RESOLVED", "IGNORED"], RESOLVED: ["OPEN"], IGNORED: ["OPEN"] };
  return <>
    <PageHeader title="Incident investigation" action={<Link className="back-link" to="/incidents"><ArrowLeft size={16} /> Back to incidents</Link>}>{incidentID}</PageHeader>
    <Panel title={record.title} meta={`first seen ${formatTime(record.first_seen_at)}`}><div className="incident-summary"><Fact label="Status"><Badge value={record.status} /></Fact><Fact label="Severity"><Badge value={record.severity} /></Fact><Fact label="Occurrences">{record.occurrence_count}</Fact><Fact label="Service">{record.primary_service}</Fact><Fact label="Last seen">{formatTime(record.last_seen_at)}</Fact>{record.error_message && <Fact label="Error">{record.error_message}</Fact>}{record.trace_id && <Fact label="Trace"><Link className="back-link" to={`/traces/${record.trace_id}`}>Open trace</Link></Fact>}</div><div className="incident-actions"><span className="muted">Change status</span>{(transitions[record.status] || []).map((status) => <button disabled={mutation.isPending} onClick={() => mutation.mutate(status)} key={status}>{status}</button>)}{mutation.isError && <span className="message error">{mutation.error.message}</span>}</div></Panel>
    <Panel title="Occurrence timeline" meta={`${occurrenceItems.length} records`}><DataTable headers={["Time", "Service", "Operation", "Error", "Trace"]} empty="No occurrences are retained for this incident." rows={occurrenceItems.map((item) => [formatTime(item.occurred_at), item.service_name, item.operation || "—", item.error_message || item.error_type || "—", item.trace_id ? <Link className="table-link" to={`/traces/${item.trace_id}`}>Open trace</Link> : "—"])} /></Panel>
    <Panel title="Linked telemetry" meta={`${eventItems.length} records`}><DataTable headers={["Time", "Type", "Service", "Operation", "Error"]} empty="No telemetry events are linked to this incident." rows={eventItems.map((item) => [formatTime(item.occurred_at), item.event_type, item.service_name, item.operation || "—", item.error_message || item.error_type || "—"])} /></Panel>
    <Panel title="Status history" meta={`${changeItems.length} changes`}><DataTable headers={["Changed", "From", "To", "Actor"]} empty="No status changes have been recorded." rows={changeItems.map((item) => [formatTime(item.changed_at), <Badge value={item.previous_status} />, <Badge value={item.status} />, item.actor_key_id || "system"])} /></Panel>
  </>;
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return <div className="incident-fact"><span>{label}</span><strong>{children}</strong></div>;
}

function object(value: unknown): Record<string, string> {
  try {
    const parsed = typeof value === "string" ? JSON.parse(value) : value;
    return parsed && typeof parsed === "object" ? parsed as Record<string, string> : {};
  } catch {
    return {};
  }
}

type PagedQuery = { isPending: boolean; isError: boolean; error: unknown; refetch: () => Promise<unknown>; hasNextPage: boolean; isFetchingNextPage: boolean; fetchNextPage: () => Promise<unknown> };

function PagedTable({ query, headers, rows, empty }: { query: PagedQuery; headers: string[]; rows: ReactNode[][]; empty: string }) {
  if (query.isPending) return <Loading />;
  if (query.isError) return <ErrorState error={query.error} retry={() => void query.refetch()} />;
  return <><DataTable headers={headers} rows={rows} empty={empty} /><LoadMore hasMore={query.hasNextPage} loading={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()} /></>;
}
