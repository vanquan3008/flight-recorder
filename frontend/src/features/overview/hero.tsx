import { Link } from "react-router-dom";
import type { Incident, RequestPerformance } from "../../shared/types";
import { Badge, DetailLink, formatDuration } from "../../shared/ui";
import { PerformanceTrend } from "../performance/trend";

export type StatusTone = "danger" | "warning" | "ok" | "idle";
export type OverviewStatus = { tone: StatusTone; headline: string; detail: string };

const shownIncidents = 3;
const percent = (value: number) => `${(value * 100).toFixed(value > 0 && value < 0.001 ? 2 : 1)}%`;
const plural = (value: number, noun: string) => `${value.toLocaleString()} ${noun}${value === 1 ? "" : "s"}`;

// One sentence about the window's traffic, or nothing when there was none to describe.
export function trafficSummary(performance?: RequestPerformance): string {
  const summary = performance?.summary;
  if (!summary || summary.requests === 0) return "";
  return `${plural(summary.requests, "request")} · ${percent(summary.error_rate)} errors · p95 ${formatDuration(summary.p95_latency_nanos)}`;
}

// The page leads with the worst thing an operator should know: open incidents, then failing traces, then calm.
export function overviewStatus({ openIncidents, failedTraces, performance }: { openIncidents: number; failedTraces: number; performance?: RequestPerformance }): OverviewStatus {
  const traffic = trafficSummary(performance);
  if (openIncidents > 0) return { tone: "danger", headline: `${plural(openIncidents, "incident")} ${openIncidents === 1 ? "needs" : "need"} attention`, detail: traffic };
  if (failedTraces > 0) return { tone: "warning", headline: "No open incidents, but traces are failing", detail: `${plural(failedTraces, "failed trace")} in the latest page${traffic ? ` · ${traffic}` : ""}` };
  if (traffic) return { tone: "ok", headline: "All systems normal", detail: traffic };
  return { tone: "idle", headline: "No inbound traffic in this window", detail: "Widen the time range or send a request from an instrumented application." };
}

export function StatusHero({ status, incidents, performance, pending, filters }: { status: OverviewStatus; incidents: Incident[]; performance?: RequestPerformance; pending: boolean; filters: string }) {
  return <section className={`status-hero tone-${status.tone}`} aria-label="System status">
    <div className="status-copy">
      <span className="status-eyebrow"><i className="status-dot" aria-hidden="true" />Status</span>
      <h2>{status.headline}</h2>
      {status.detail && <p>{status.detail}</p>}
      {incidents.length > 0 ? <ul className="status-incidents">
        {incidents.slice(0, shownIncidents).map((incident) => <li key={incident.id}>
          <Badge value={incident.severity} /><span className="status-incident-text"><span className="status-incident-title">{incident.title}</span><small>{incident.primary_service}</small></span>
          <DetailLink to={`/incidents/${incident.id}`}>Investigate</DetailLink>
        </li>)}
        {incidents.length > shownIncidents && <li className="status-more"><Link to={`/incidents?${filters}`}>All {incidents.length} incidents →</Link></li>}
      </ul> : <Link className="status-cta" to={`/traces?${filters}`}>Explore traces →</Link>}
    </div>
    <div className="status-chart">
      {pending ? <p className="empty">Loading traffic…</p> : performance ? <PerformanceTrend series={performance.series} kind="rate" /> : <p className="empty">Traffic is unavailable right now.</p>}
      <Link className="status-chart-link" to={`/performance?${filters}`}>Performance details →</Link>
    </div>
  </section>;
}
