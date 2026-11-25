import { secondaryButtonClass, primaryButtonClass } from "../../shared/ui/styles";
import { Link, useLocation } from "react-router-dom";
import type { Incident, RequestPerformance } from "../../shared/types";
import { Badge, DetailLink, formatCount, formatDuration, formatPercent } from "../../shared/ui";
import { contextURL } from "../../shared/ui/links";
import { PerformanceTrend } from "../performance/trend";

type StatusTone = "danger" | "warning" | "ok" | "idle";
type OverviewStatus = { tone: StatusTone; headline: string; detail: string };

const shownIncidents = 3;

// One sentence about the window's traffic, or nothing when there was none to describe.
export function trafficSummary(performance?: RequestPerformance): string {
  const summary = performance?.summary;
  if (!summary || summary.requests === 0) return "";
  return `${formatCount(summary.requests, "request")} · ${formatPercent(summary.error_rate)} errors · p95 ${formatDuration(summary.p95_latency_nanos)}`;
}

// The page leads with the worst thing an operator should know: open incidents, then failing traces, then calm.
export function overviewStatus({ openIncidents, failedTraces, performance, incidentsIncomplete = false }: { openIncidents: number; failedTraces: number; performance?: RequestPerformance; incidentsIncomplete?: boolean }): OverviewStatus {
  const traffic = trafficSummary(performance);
  if (openIncidents > 0) return { tone: "danger", headline: `${formatCount(openIncidents, "incident")} ${openIncidents === 1 ? "needs" : "need"} attention`, detail: `Active incidents on the latest page${traffic ? ` · ${traffic}` : ""}` };
  if (failedTraces > 0) return { tone: "warning", headline: "Errors observed in recent traces", detail: `${formatCount(failedTraces, "failed trace")} in the latest page${traffic ? ` · ${traffic}` : ""}` };
  if (performance && performance.summary.errors > 0) return { tone: "warning", headline: "Errors observed in this window", detail: traffic };
  if (incidentsIncomplete) return { tone: "warning", headline: "More incidents are outside this page", detail: "Open the incident explorer and filter by status to review active incidents." };
  if (traffic) return { tone: "ok", headline: "No request errors observed", detail: `${traffic} · Incident summary covers the latest page.` };
  if (!performance) return { tone: "idle", headline: "Traffic status is unavailable", detail: "The performance query has not returned a result. Retry or check the Performance page." };
  return { tone: "idle", headline: "No inbound traffic in this window", detail: "Widen the time range or send a request from an instrumented application." };
}

export function StatusHero({ status, incidents, performance, pending, filters }: { status: OverviewStatus; incidents: Incident[]; performance?: RequestPerformance; pending: boolean; filters: string }) {
  const location = useLocation();
  const toneClasses = {
    danger: "border-danger/35 from-danger/15",
    warning: "border-warning/35 from-warning/15",
    ok: "border-svc-1/35 from-svc-1/15",
    idle: "border-faint/35 from-faint/15",
  }[status.tone];
  const dotClasses = {
    danger: "bg-danger shadow-[0_0_12px_var(--danger)]",
    warning: "bg-warning shadow-[0_0_12px_var(--warning)]",
    ok: "bg-svc-1 shadow-[0_0_12px_var(--svc-1)]",
    idle: "bg-faint shadow-[0_0_12px_var(--faint)]",
  }[status.tone];

  return <section className={`status-hero border border-glass-edge bg-glass backdrop-glass shadow-glass mb-4 grid grid-cols-1 overflow-hidden rounded-[20px] bg-radial-[at_0%_0%] to-transparent min-[1041px]:grid-cols-[minmax(0,1.4fr)_minmax(320px,1fr)] ${toneClasses} [&_h2]:m-0 [&_h2]:text-[22px] [&_h2]:leading-tight [&_h2]:font-bold tone-${status.tone}`} aria-label="System status">
    <div className="status-copy flex min-w-0 flex-col justify-center gap-2 px-6 py-5 [&>p]:m-0 [&>p]:text-[12.5px] [&>p]:font-medium [&>p]:text-muted">
      <span className="status-eyebrow inline-flex items-center gap-1.75 text-[11px] font-bold tracking-widest text-muted uppercase"><i className={`status-dot size-2 rounded-full ${dotClasses}`} aria-hidden="true" />Status</span>
      <h2>{status.headline}</h2>
      {status.detail && <p>{status.detail}</p>}
      {incidents.length > 0 ? <ul className="status-incidents mt-2 mb-0 grid list-none gap-1.75 p-0 [&>li]:grid [&>li]:grid-cols-[auto_minmax(0,1fr)_auto] [&>li]:items-center [&>li]:gap-2.5 [&>li]:rounded-xl [&>li]:border [&>li]:border-glass-divider [&>li]:bg-glass-nested [&>li]:px-3 [&>li]:py-1.75 [&>li]:text-xs [&>li:hover]:bg-glass-hover">
        {incidents.slice(0, shownIncidents).map((incident) => <li key={incident.id}>
          <Badge value={incident.severity} /><span className="status-incident-text grid min-w-0 [&>small]:text-[11px] [&>small]:font-medium [&>small]:text-faint"><span className="status-incident-title truncate font-semibold">{incident.title}</span><small>{incident.primary_service}</small></span>
          <DetailLink to={`/incidents/${incident.id}`} className={`status-investigate-pill ${secondaryButtonClass} rounded-full px-2.5 py-0.75 text-[11px] text-accent no-underline`}>Investigate</DetailLink>
        </li>)}
        {incidents.length > shownIncidents && <li className="status-more block border-0 bg-transparent px-1 py-0 [&_a]:text-[11.5px] [&_a]:font-semibold [&_a]:text-accent [&_a]:no-underline"><Link to={contextURL(`/incidents?${filters}`, location.search, true)}>All {incidents.length} incidents →</Link></li>}
      </ul> : <Link className={`status-cta ${primaryButtonClass} mt-2 self-start no-underline`} to={contextURL(`/traces?${filters}`, location.search, true)}>Explore traces →</Link>}
    </div>
    <div className="status-chart flex min-w-0 flex-col justify-between border-t border-glass-divider bg-surface/22 px-4.5 pt-3.5 pb-2.5 min-[1041px]:border-t-0 min-[1041px]:border-l [&>.performance-trend]:border-0 [&>.performance-trend]:bg-transparent [&>.performance-trend]:p-0 [&_.empty]:py-9">
      {pending ? <p className="empty m-0 p-8 text-center text-sm text-muted">Loading traffic…</p> : performance ? <PerformanceTrend series={performance.series} kind="rate" /> : <p className="empty m-0 p-8 text-center text-sm text-muted">Traffic is unavailable right now.</p>}
      <Link className="status-chart-link inline-flex items-center gap-1 self-end px-0.5 pt-0.5 pb-1 text-[11.5px] font-semibold text-accent no-underline hover:underline" to={contextURL(`/performance?${filters}`, location.search, true)}>Performance details →</Link>
    </div>
  </section>;
}
