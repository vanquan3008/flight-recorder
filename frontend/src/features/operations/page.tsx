import { Search } from "lucide-react";
import type { FormEvent } from "react";
import { useSearchParams } from "react-router-dom";
import { useAuth } from "../../app/auth";
import { compactSearchParams, pageItems, queryString, usePagedQuery } from "../../shared/api";
import type { Incident } from "../../shared/types";
import { Badge, DetailLink, ErrorState, PageHeader, PagedTable, Panel, formatTime } from "../../shared/ui";

export function OperationsPage() {
  const { credential } = useAuth();
  const [params, setParams] = useSearchParams();
  const path = `/api/v1/incidents?${queryString({ service: params.get("service") || undefined, status: params.get("status") || undefined, from: params.get("from") || undefined, to: params.get("to") || undefined })}`;
  const query = usePagedQuery<Incident>(["incidents"], path, credential);
  const incidents = pageItems(query.data?.pages);
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setParams(compactSearchParams(event.currentTarget, params, ["from", "to"]));
  };
  if (query.isError) return <ErrorState error={query.error} retry={() => void query.refetch()} fullPage />;
  return <><PageHeader title="Incidents">Investigate application failures from their timeline and correlated telemetry.</PageHeader><form className="toolbar" onSubmit={submit}><input name="service" placeholder="Service" aria-label="Filter by service" defaultValue={params.get("service") || ""} /><select name="status" aria-label="Filter by status" defaultValue={params.get("status") || ""}><option value="">All statuses</option><option>OPEN</option><option>INVESTIGATING</option><option>RESOLVED</option><option>IGNORED</option></select><button><Search size={16} /> Apply filters</button></form><Panel title="Incident timeline" meta={query.isPending ? undefined : `${incidents.length} loaded`}><PagedTable query={query} empty="No incidents yet." headers={["Last seen", "Severity", "Status", "Title", "Occurrences", ""]} rows={incidents.map((incident) => [
    formatTime(incident.last_seen_at), <Badge value={incident.severity} />, <Badge value={incident.status} />, incident.title, incident.occurrence_count, <DetailLink to={`/incidents/${incident.id}`}>Inspect</DetailLink>,
  ])} /></Panel></>;
}

export { IncidentPage } from "./incident-page";
