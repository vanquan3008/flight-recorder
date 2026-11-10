import { useQueries, useQuery } from "@tanstack/react-query";
import { Activity, AlertTriangle, BellRing, Gauge } from "lucide-react";
import { Link } from "react-router-dom";
import { useAuth } from "../../app/auth";
import { api, queryString } from "../../shared/api";
import type { Incident, Page, RequestPerformance, TelemetryQuality, TraceSummary } from "../../shared/types";
import { Badge, DataTable, DetailLink, ErrorState, Loading, PageHeader, Panel, TimeWindowFilter, formatDuration, formatTime, useTimeWindow } from "../../shared/ui";
import { Onboarding } from "../onboarding/onboarding";
import { StatusHero, overviewStatus } from "./hero";

const latest = <T,>(path: string, credential: string) => ({
  queryKey: ["overview", path],
  queryFn: ({ signal }: { signal: AbortSignal }) => api<Page<T>>(path, credential, { signal }),
});

export function OverviewPage() {
  const { credential, session } = useAuth();
  const window = useTimeWindow();
  const filters = queryString({ from: window.from, to: window.to });
  const qualityPath = `/api/v1/telemetry/quality?${filters}`;
  const [incidents, quality, traces] = useQueries({
    queries: [
      latest<Incident>(`/api/v1/incidents?${filters}`, credential),
      { queryKey: ["overview", qualityPath], queryFn: ({ signal }: { signal: AbortSignal }) => api<TelemetryQuality>(qualityPath, credential, { signal }) },
      latest<TraceSummary>(`/api/v1/traces?${filters}`, credential),
    ],
  });
  // Same path as the details section below when it has no filter, so both read one request.
  const performancePath = `/api/v1/performance?${queryString({ from: window.from, to: window.to })}`;
  const performance = useQuery({ queryKey: ["performance", performancePath], queryFn: ({ signal }) => api<RequestPerformance>(performancePath, credential, { signal }) });
  const queries = [incidents, quality, traces];
  if (queries.some((query) => query.isPending)) return <Loading page />;
  const failed = queries.find((query) => query.isError);
  if (failed) return <ErrorState error={failed.error} retry={() => queries.forEach((query) => void query.refetch())} />;

  const loadedIncidents = incidents.data?.data || [];
  const loadedTraces = traces.data?.data || [];
  const actionable = loadedIncidents.filter((incident) => incident.status === "OPEN" || incident.status === "INVESTIGATING");
  const cards = [
    { label: "Loaded active incidents", value: actionable.length, path: `/incidents?${filters}`, hint: `open or investigating · ${loadedIncidents.length} incidents on this page (max 50)`, Icon: BellRing, color: "text-rose-400 bg-rose-500/10" },
    { label: "Loaded traces", value: loadedTraces.length, path: `/traces?${filters}`, hint: "latest page in window · max 50", Icon: Activity, color: "text-sky-400 bg-sky-500/10" },
    { label: "Loaded failed traces", value: loadedTraces.filter((trace) => trace.error_count > 0).length, path: `/traces?${filters}&status=ERROR`, hint: "with an error span · latest page only", Icon: AlertTriangle, color: "text-amber-400 bg-amber-500/10" },
    { label: "Metric points", value: quality.data?.metric_points || 0, hint: "all stored points in window", Icon: Gauge, color: "text-emerald-400 bg-emerald-500/10" },
  ];
  const hasTrace = Boolean(traces.data?.data.length);
  const failedTraces = loadedTraces.filter((trace) => trace.error_count > 0).length;
  const status = overviewStatus({ openIncidents: actionable.length, failedTraces, performance: performance.data, incidentsIncomplete: Boolean(incidents.data?.next_cursor) });
  // With no trace and no traffic there is nothing to summarise yet: the setup steps lead instead.
  const showHero = hasTrace || status.tone !== "idle";
  // Four zeros and an empty list say nothing: until there is data, only the setup steps lead.
  const quiet = !hasTrace && cards.every((card) => !card.value);
  return <div className="overview-page flex min-h-0 flex-1 flex-col">
    <div className="shrink-0">
      <PageHeader title="Telemetry overview" action={<TimeWindowFilter {...window} />}>Start with a failed trace, inspect its spans, then follow the correlated logs.</PageHeader>
      <Onboarding credential={credential} projectID={session?.project?.project.id ?? ""} canCreate={session?.role === "owner"} hasTrace={hasTrace} />
      {showHero && <StatusHero status={status} incidents={actionable} performance={performance.data} pending={performance.isPending} filters={filters} />}
      {!quiet && <div className="stat-grid mb-3.5 grid grid-cols-2 overflow-hidden rounded-[18px] border border-glass-edge bg-glass shadow-glass backdrop-glass min-[901px]:grid-cols-4">{cards.map((card) => {
        const { Icon, color } = card;
        const content = <>
          <div className="flex items-center justify-between mb-1">
            <span className="text-[10px] font-semibold uppercase tracking-wider text-slate-600 dark:text-slate-300">{card.label}</span>
            <span className={`p-1 rounded-md ${color}`}><Icon size={14} /></span>
          </div>
          <strong className="block text-2xl font-black tracking-tight text-slate-950 dark:text-white my-0.5">{card.value}</strong>
          <span title={card.hint} className="truncate text-[11px] font-medium text-faint">{card.hint}</span>
        </>;
        const cardClasses = "stat-card flex min-w-0 flex-col justify-between border-r border-glass-divider px-4 py-3 text-inherit no-underline transition-colors hover:bg-glass-hover last:border-r-0 max-[900px]:even:border-r-0 max-[900px]:nth-[-n+2]:border-b";
        return card.path ? <Link className={cardClasses} to={card.path} key={card.label}>{content}</Link> : <div className={cardClasses} key={card.label}>{content}</div>;
      })}</div>}
    </div>

    {!quiet && <Panel fit title="Recent traces" meta={<Link to={`/traces?${filters}`}>View all traces →</Link>}><DataTable
      headers={["Started", "Service", "Duration", "Spans", "Status", ""]}
      empty="No traces yet. Complete the setup steps above to send the first one."
      rows={(traces.data?.data || []).slice(0, 10).map((trace) => [
        formatTime(trace.started_at), trace.root_service, formatDuration(trace.duration_nanos), trace.span_count, <Badge value={trace.error_count ? "ERROR" : "OK"} />, <DetailLink to={`/traces/${trace.trace_id}`}>Inspect</DetailLink>,
      ])}
    /></Panel>}
  </div>;
}
