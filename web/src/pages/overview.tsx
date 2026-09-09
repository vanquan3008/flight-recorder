import { useQueries } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "../api";
import { useAuth } from "../auth";
import type { Detection, Incident, LogRecord, Page, SecurityEvent } from "../types";
import { Badge, DataTable, ErrorState, Loading, PageHeader, Panel, formatTime } from "../ui";

const latest = <T,>(path: string, apiKey: string) => ({
  queryKey: ["overview", path],
  queryFn: ({ signal }: { signal: AbortSignal }) => api<Page<T>>(path, apiKey, { signal }),
});

export function OverviewPage() {
  const { apiKey } = useAuth();
  const [events, detections, incidents, logs] = useQueries({
    queries: [
      latest<SecurityEvent>("/api/v1/security/events?limit=50", apiKey),
      latest<Detection>("/api/v1/security/detections?limit=50", apiKey),
      latest<Incident>("/api/v1/incidents?limit=50", apiKey),
      latest<LogRecord>("/api/v1/logs?limit=50", apiKey),
    ],
  });
  const queries = [events, detections, incidents, logs];
  if (queries.some((query) => query.isPending)) return <Loading />;
  const failed = queries.find((query) => query.isError);
  if (failed) return <ErrorState error={failed.error} retry={() => queries.forEach((query) => void query.refetch())} />;

  const cards = [
    ["Security events", events.data?.data.length || 0, "/events"],
    ["Detections", detections.data?.data.length || 0, "/detections"],
    ["Incidents", incidents.data?.data.length || 0, "/incidents"],
    ["Telemetry logs", logs.data?.data.length || 0, "/machine-logs"],
  ];
  return <>
    <PageHeader title="Security overview">A quick read on your most recent security and telemetry activity.</PageHeader>
    <div className="stat-grid">{cards.map(([label, value, path]) => <Link className="stat-card" to={path as string} key={label as string}><span className="stat-label">{label}</span><strong className="stat-value">{value}</strong><span className="stat-hint">latest page · max 50</span></Link>)}</div>
    <Panel title="Recent security activity" meta="latest 10"><DataTable
      headers={["Time", "Kind", "Source", "Destination", "Outcome", "Severity"]}
      empty="No security events have arrived yet."
      rows={(events.data?.data || []).slice(0, 10).map((event) => [
        formatTime(event.occurred_at), event.kind, event.source_ip || "—", event.destination_ip || "—", <Badge value={event.outcome} />, <Badge value={event.severity} />,
      ])}
    /></Panel>
  </>;
}
