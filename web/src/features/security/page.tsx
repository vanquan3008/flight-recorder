import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, Search } from "lucide-react";
import type { FormEvent } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { useAuth } from "../../app/auth";
import { api, pageItems, queryString, usePagedQuery } from "../../shared/api";
import { compactSearchParams } from "../../shared/search-params";
import { PagedTable } from "../../shared/paged-table";
import type { Detection, LogRecord, Page, SecurityEvent, SecuritySource } from "../../shared/types";
import { Badge, DataTable, ErrorState, Loading, PageHeader, Panel, formatTime, json } from "../../shared/ui";

export function SecurityEventsPage() {
  const { apiKey } = useAuth();
  const [params, setParams] = useSearchParams();
  const path = `/api/v1/security/events?${queryString({ sourceIp: params.get("sourceIp") || undefined, outcome: params.get("outcome") || undefined })}`;
  const query = usePagedQuery<SecurityEvent>(["security-events"], path, apiKey);
  const events = pageItems(query.data?.pages);
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setParams(compactSearchParams(event.currentTarget));
  };
  return <>
    <PageHeader title="Security events">Normalized Syslog, CEF, LEEF and JSON records with raw evidence links.</PageHeader>
    <form className="toolbar" onSubmit={submit}><input name="sourceIp" placeholder="Source IP" aria-label="Filter by source IP" defaultValue={params.get("sourceIp") || ""} /><input name="outcome" placeholder="Outcome" aria-label="Filter by outcome" defaultValue={params.get("outcome") || ""} /><button><Search size={16} /> Apply filters</button></form>
    <Panel title="Event stream" meta={`${events.length} loaded`}><PagedTable query={query} empty="No events match those filters." headers={["Time", "Format", "Kind", "Source", "Destination", "Action", "Outcome", "Severity", "Message", ""]} rows={events.map((event) => [
      formatTime(event.occurred_at), event.format, event.kind, event.source_ip || "—", event.destination_ip || "—", event.action || "—", <Badge value={event.outcome} />, <Badge value={event.severity} />, event.message || "—", <Link className="table-link" to={`/events/${event.id}`}>Inspect</Link>,
    ])} /></Panel>
  </>;
}

export function SecurityEventPage() {
  const { eventID = "" } = useParams();
  const { apiKey } = useAuth();
  const event = useQuery({ queryKey: ["security-event", eventID], queryFn: ({ signal }) => api<SecurityEvent>(`/api/v1/security/events/${eventID}`, apiKey, { signal }) });
  const raw = useQuery({ enabled: Boolean(event.data?.raw_event_id), queryKey: ["security-event-raw", event.data?.raw_event_id], queryFn: ({ signal }) => api<LogRecord>(`/api/v1/logs/${event.data?.raw_event_id}`, apiKey, { signal }) });
  if (event.isPending) return <Loading />;
  if (event.isError) return <ErrorState error={event.error} retry={() => void event.refetch()} />;
  const details = event.data;
  return <>
    <PageHeader title="Security event evidence" action={<Link className="back-link" to="/events"><ArrowLeft size={16} /> Back to security events</Link>}>{eventID}</PageHeader>
    <Panel title="Normalized event"><DataTable headers={["Field", "Value"]} rows={[
      ["Time", formatTime(details.occurred_at)], ["Format", details.format], ["Kind", details.kind], ["Source", details.source_ip || "—"], ["Destination", details.destination_ip || "—"], ["Action", details.action || "—"], ["Outcome", <Badge value={details.outcome} />], ["Severity", <Badge value={details.severity} />], ["Host", details.host_name || "—"], ["User", details.user_name || "—"], ["Message", details.message || "—"],
    ]} /></Panel>
    <Panel title="Immutable raw evidence" meta={details.raw_event_id}>{raw.isPending ? <Loading /> : raw.isError ? <ErrorState error={raw.error} retry={() => void raw.refetch()} /> : <pre className="raw-evidence">{json(raw.data?.body)}</pre>}</Panel>
  </>;
}

export function DetectionsPage() {
  const { apiKey } = useAuth();
  const query = usePagedQuery<Detection>(["detections"], "/api/v1/security/detections?limit=50", apiKey);
  const detections = pageItems(query.data?.pages);
  return <><PageHeader title="Detections">Rule matches are evaluated after raw logs are durable.</PageHeader><Panel title="Detection history" meta={`${detections.length} loaded`}><PagedTable query={query} empty="No detections yet." headers={["Created", "Severity", "Rule", "Security event", "Incident", "Group", "Window"]} rows={detections.map((detection) => [
    formatTime(detection.created_at), <Badge value={detection.severity} />, <code>{detection.rule_id}</code>, <Link className="table-link" to={`/events/${detection.security_event_id}`}>Open event</Link>, detection.incident_id ? <Link className="table-link" to={`/incidents/${detection.incident_id}`}>Open incident</Link> : "—", detection.group_key || "—", `${formatTime(detection.window_started_at)} → ${formatTime(detection.window_ended_at)}`,
  ])} /></Panel></>;
}

export function SourcesPage() {
  const { apiKey } = useAuth();
  const query = useQuery({ queryKey: ["sources"], queryFn: ({ signal }) => api<Page<SecuritySource>>("/api/v1/security/sources?limit=100", apiKey, { signal }) });
  if (query.isPending) return <Loading />;
  if (query.isError) return <ErrorState error={query.error} retry={() => void query.refetch()} />;
  const sources = query.data.data;
  return <><PageHeader title="Sources">Health and volume of security senders observed by TracePilot.</PageHeader><Panel title="Source inventory" meta={`${sources.length} sources`}><DataTable headers={["Name", "Type", "Format", "Last seen", "Accepted", "Errors"]} empty="No security source has been observed yet." rows={sources.map((source) => [source.name, source.type, source.format, formatTime(source.last_seen_at), source.accepted_count, source.error_count])} /></Panel></>;
}
