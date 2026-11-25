import { Activity, AlertTriangle, BellRing, Gauge } from "lucide-react";
import { Link, useLocation } from "react-router-dom";
import { useAuth } from "../../shared/session";
import {
  Badge,
  DataTable,
  DetailLink,
  ErrorState,
  Loading,
  PageHeader,
  Panel,
  TimeWindowFilter,
  formatDuration,
  formatTime,
  useTimeWindow,
} from "../../shared/ui";
import { contextURL } from "../../shared/ui/links";
import { Onboarding } from "../onboarding/onboarding";
import { StatusHero, overviewStatus } from "./hero";
import { useOverviewData } from "./use-overview-data";

export function OverviewPage() {
  const { session } = useAuth();
  const window = useTimeWindow();
  const location = useLocation();
  const {
    credential,
    filters,
    incidents,
    traces,
    hasNextIncident,
    metricPoints,
    performance,
    performancePending,
    isPending,
    error,
    refetch,
  } = useOverviewData(window.from, window.to);

  if (isPending) return <Loading page />;
  if (error) return <ErrorState error={error} retry={refetch} />;

  const actionable = incidents.filter(
    (i) => i.status === "OPEN" || i.status === "INVESTIGATING",
  );
  const failedTraces = traces.filter((t) => t.error_count > 0).length;
  const cards = [
    {
      label: "Loaded active incidents",
      value: actionable.length,
      path: `/incidents?${filters}`,
      hint: `open or investigating · ${incidents.length} incidents on this page (max 50)`,
      Icon: BellRing,
      color: "text-rose-400 bg-rose-500/10",
    },
    {
      label: "Loaded traces",
      value: traces.length,
      path: `/traces?${filters}`,
      hint: "latest page in window · max 50",
      Icon: Activity,
      color: "text-sky-400 bg-sky-500/10",
    },
    {
      label: "Loaded failed traces",
      value: failedTraces,
      path: `/traces?${filters}&status=ERROR`,
      hint: "with an error span · latest page only",
      Icon: AlertTriangle,
      color: "text-amber-400 bg-amber-500/10",
    },
    {
      label: "Metric points",
      value: metricPoints,
      hint: "all stored points in window",
      Icon: Gauge,
      color: "text-emerald-400 bg-emerald-500/10",
    },
  ];
  const hasTrace = traces.length > 0;
  const status = overviewStatus({
    openIncidents: actionable.length,
    failedTraces,
    performance,
    incidentsIncomplete: hasNextIncident,
  });
  const showHero = hasTrace || status.tone !== "idle";
  const quiet = !hasTrace && cards.every((card) => !card.value);

  return (
    <div className="overview-page flex min-h-0 flex-1 flex-col">
      <div className="shrink-0">
        <PageHeader
          title="Telemetry overview"
          action={<TimeWindowFilter {...window} />}
        >
          Start with a failed trace, inspect its spans, then follow the correlated logs.
        </PageHeader>
        <Onboarding
          credential={credential}
          projectID={session?.project?.project.id ?? ""}
          environment={session?.environment ?? ""}
          canCreate={session?.role === "owner"}
          hasTrace={hasTrace}
        />
        {showHero && (
          <StatusHero
            status={status}
            incidents={actionable}
            performance={performance}
            pending={performancePending}
            filters={filters}
          />
        )}
        {!quiet && (
          <div className="stat-grid mb-3.5 grid grid-cols-2 overflow-hidden rounded-[18px] border border-glass-edge bg-glass shadow-glass backdrop-glass min-[901px]:grid-cols-4">
            {cards.map((card) => {
              const { Icon, color } = card;
              const content = (
                <>
                  <div className="mb-1 flex items-center justify-between">
                    <span className="text-[10px] font-semibold uppercase tracking-wider text-slate-600 dark:text-slate-300">
                      {card.label}
                    </span>
                    <span className={`rounded-md p-1 ${color}`}>
                      <Icon size={14} />
                    </span>
                  </div>
                  <strong className="my-0.5 block text-2xl font-black tracking-tight text-slate-950 dark:text-white">
                    {card.value}
                  </strong>
                  <span title={card.hint} className="truncate text-[11px] font-medium text-faint">
                    {card.hint}
                  </span>
                </>
              );
              const cardClasses =
                "stat-card flex min-w-0 flex-col justify-between border-r border-glass-divider px-4 py-3 text-inherit no-underline transition-colors hover:bg-glass-hover last:border-r-0 max-[900px]:even:border-r-0 max-[900px]:nth-[-n+2]:border-b";
              return card.path ? (
                <Link
                  className={cardClasses}
                  to={contextURL(card.path, location.search, true)}
                  key={card.label}
                >
                  {content}
                </Link>
              ) : (
                <div className={cardClasses} key={card.label}>
                  {content}
                </div>
              );
            })}
          </div>
        )}
      </div>

      {!quiet && (
        <Panel
          fit
          title="Recent traces"
          meta={
            <Link to={contextURL(`/traces?${filters}`, location.search, true)}>
              View all traces →
            </Link>
          }
        >
          <DataTable
            headers={["Started", "Service", "Duration", "Spans", "Status", ""]}
            empty="No traces yet. Complete the setup steps above to send the first one."
            rows={traces.slice(0, 10).map((trace) => [
              formatTime(trace.started_at),
              trace.root_service,
              formatDuration(trace.duration_nanos),
              trace.span_count,
              <Badge key={`status-${trace.trace_id}`} value={trace.error_count ? "ERROR" : "OK"} />,
              <DetailLink key={`detail-${trace.trace_id}`} to={`/traces/${trace.trace_id}`}>Inspect</DetailLink>,
            ])}
          />
        </Panel>
      )}
    </div>
  );
}
