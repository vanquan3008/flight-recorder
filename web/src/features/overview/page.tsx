import { useQueries } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { DetailLink } from "../../shared/navigation";
import { useAuth } from "../../app/auth";
import { api, queryString } from "../../shared/api";
import { TimeWindowFilter, useTimeWindow } from "../../shared/time-window";
import type { Incident, Page, TelemetryQuality, TraceSummary } from "../../shared/types";
import { Badge, DataTable, ErrorState, Loading, PageHeader, Panel, formatDuration, formatTime } from "../../shared/ui";
import { Onboarding } from "../onboarding/onboarding";
import { PerformanceDashboard } from "../performance/dashboard";

const latest = <T,>(path: string, apiKey: string) => ({
  queryKey: ["overview", path],
  queryFn: ({ signal }: { signal: AbortSignal }) => api<Page<T>>(path, apiKey, { signal }),
});

export function OverviewPage() {
  const { apiKey } = useAuth();
  const window = useTimeWindow();
  const filters = queryString({ from: window.from, to: window.to });
  const qualityPath = `/api/v1/telemetry/quality?${filters}`;
  const [incidents, quality, traces] = useQueries({
    queries: [
      latest<Incident>(`/api/v1/incidents?${filters}`, apiKey),
      { queryKey: ["overview", qualityPath], queryFn: ({ signal }: { signal: AbortSignal }) => api<TelemetryQuality>(qualityPath, apiKey, { signal }) },
      latest<TraceSummary>(`/api/v1/traces?${filters}`, apiKey),
    ],
  });
  const queries = [incidents, quality, traces];
  if (queries.some((query) => query.isPending)) return <Loading />;
  const failed = queries.find((query) => query.isError);
  if (failed) return <ErrorState error={failed.error} retry={() => queries.forEach((query) => void query.refetch())} />;

  const loadedIncidents = incidents.data?.data || [];
  const loadedTraces = traces.data?.data || [];
  const actionable = loadedIncidents.filter((incident) => incident.status === "OPEN" || incident.status === "INVESTIGATING");
  const cards = [
    { label: "Incidents needing action", value: actionable.length, path: `/incidents?${filters}`, hint: `open or investigating · ${loadedIncidents.length} incidents on this page (max 50)` },
    { label: "Traces", value: loadedTraces.length, path: `/traces?${filters}`, hint: "latest page in window · max 50" },
    { label: "Failed traces", value: loadedTraces.filter((trace) => trace.error_count > 0).length, path: `/traces?${filters}&status=ERROR`, hint: "with an error span · latest page only" },
    { label: "Metric points", value: quality.data?.metric_points || 0, hint: "all stored points in window" },
  ];
  return <>
    <PageHeader title="Telemetry overview">Start with a failed trace, inspect its spans, then follow the correlated logs.</PageHeader>
    <TimeWindowFilter range={window.range} select={window.select} />
    <Onboarding apiKey={apiKey} hasTrace={Boolean(traces.data?.data.length)} />
    <div className="stat-grid">{cards.map((card) => {
      const content = <><span className="stat-label">{card.label}</span><strong className="stat-value">{card.value}</strong><span className="stat-hint">{card.hint}</span></>;
      return card.path ? <Link className="stat-card" to={card.path} key={card.label}>{content}</Link> : <div className="stat-card" key={card.label}>{content}</div>;
    })}</div>
    {actionable.length > 0 && <Panel title="Incidents needing action" meta={`${actionable.length} on this page`}><DataTable
      headers={["Last seen", "Severity", "Status", "Service", "Title", ""]}
      rows={actionable.slice(0, 5).map((incident) => [formatTime(incident.last_seen_at), <Badge value={incident.severity} />, <Badge value={incident.status} />, incident.primary_service, incident.title, <DetailLink to={`/incidents/${incident.id}`}>Investigate</DetailLink>])}
    /></Panel>}
    <PerformanceDashboard from={window.from} to={window.to} />
    <Panel title="Recent traces" meta="latest 10"><DataTable
      headers={["Started", "Service", "Duration", "Spans", "Status", ""]}
      empty="No traces yet. Complete the setup steps above to send the first one."
      rows={(traces.data?.data || []).slice(0, 10).map((trace) => [
        formatTime(trace.started_at), trace.root_service, formatDuration(trace.duration_nanos), trace.span_count, <Badge value={trace.error_count ? "ERROR" : "OK"} />, <DetailLink to={`/traces/${trace.trace_id}`}>Inspect</DetailLink>,
      ])}
    /></Panel>
  </>;
}
