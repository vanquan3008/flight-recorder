import { queryString } from "../../shared/api";
import type { PerformanceGroup, TelemetryQuality } from "../../shared/types";
import { DetailLink, Empty, formatDuration, formatPercent } from "../../shared/ui";

const topServices = 6;

// Services ranked by request errors. The bar is relative to the worst service in the window.
export function ServiceErrorBars({ services, from, to }: { services: PerformanceGroup[]; from: string; to: string }) {
  if (!services.length) return <Empty>No inbound requests in this window. Check server instrumentation and the selected filters.</Empty>;
  const ranked = [...services].sort((a, b) => b.errors - a.errors || b.requests - a.requests).slice(0, topServices);
  const worst = Math.max(...ranked.map((service) => service.errors));
  return <>
    <ul className="service-bars m-0 grid list-none gap-2.25 p-0 [&>li]:grid [&>li]:grid-cols-[minmax(80px,120px)_minmax(60px,1fr)_minmax(140px,auto)] [&>li]:items-center [&>li]:gap-3 [&>li]:text-xs [&>li]:font-semibold [&_a]:truncate [&_a]:hover:text-accent max-[620px]:[&>li]:grid-cols-[minmax(0,1fr)_auto] max-[620px]:[&_.bar-track]:col-span-full max-[620px]:[&_.bar-track]:row-start-2">{ranked.map((service) => <li key={service.service}>
      <DetailLink to={`/traces?${queryString({ service: service.service, from, to })}`}>{service.service}</DetailLink>
      <div className="bar-track h-2 overflow-hidden rounded-full bg-glass-sunken" role="img" aria-label={`${service.service}: ${service.errors} errors of ${service.requests} requests`}>
        {service.errors > 0 && <div className="bar-fill h-full rounded-[inherit] bg-red-500" style={{ width: `${Math.max(3, (service.errors / worst) * 100)}%` }} />}
      </div>
      <span className="bar-meta grid whitespace-nowrap text-right text-[11px] font-medium text-muted [&_strong]:font-semibold [&_strong]:text-text [&_small]:font-mono [&_small]:text-[10px] [&_small]:text-faint"><span><strong>{service.errors.toLocaleString()}</strong> errors · {formatPercent(service.error_rate)}</span>
        <small>{service.requests.toLocaleString()} req · p95 {service.requests ? formatDuration(service.p95_latency_nanos) : "—"}</small></span>
    </li>)}</ul>
    {worst === 0 && <p className="insight-note mt-2.5 text-[11px] text-faint">No request errors in this window.</p>}
  </>;
}

const radius = 40;
const circumference = 2 * Math.PI * radius;
const share = (part: number, whole: number) => whole > 0 ? Math.max(0, Math.min(1, part / whole)) : 0;

// How much of the telemetry can be followed: logs that carry a trace, spans that find their parent.
export function QualityDonut({ quality: reported }: { quality: TelemetryQuality }) {
  const quality = { ...{ logs: 0, uncorrelated_logs: 0, spans: 0, orphan_spans: 0 }, ...reported };
  if (quality.logs === 0 && quality.spans === 0) return <Empty>No logs or spans in this window.</Empty>;
  const correlated = share(quality.logs - quality.uncorrelated_logs, quality.logs);
  const linked = share(quality.spans - quality.orphan_spans, quality.spans);
  const label = quality.logs ? `${Math.round(correlated * 100)}%` : "—";
  return <div className="quality flex items-center gap-5 max-[620px]:flex-col max-[620px]:items-start">
    <svg className="donut size-[108px] shrink-0" viewBox="0 0 100 100" role="img" aria-label={quality.logs ? `${label} of logs carry a trace` : "No logs in this window"}>
      <circle className="donut-track stroke-glass-sunken" cx="50" cy="50" r={radius} fill="none" strokeWidth="11" />
      {quality.logs > 0 && <circle className="donut-fill stroke-sky-600" cx="50" cy="50" r={radius} fill="none" strokeWidth="11" strokeLinecap="round"
        strokeDasharray={`${circumference * correlated} ${circumference}`} transform="rotate(-90 50 50)" />}
      <text className="donut-value fill-text text-lg font-bold" x="50" y="53" textAnchor="middle">{label}</text>
      <text className="donut-caption fill-faint text-[8px] font-semibold uppercase tracking-wider" x="50" y="67" textAnchor="middle">with trace</text>
    </svg>
    <dl className="quality-rows m-0 grid min-w-0 flex-1 gap-3 [&_dt]:text-[10px] [&_dt]:font-semibold [&_dt]:uppercase [&_dt]:tracking-wider [&_dt]:text-faint [&_dd]:mt-0.5 [&_dd]:text-[12.5px] [&_dd]:font-semibold [&_dd]:tabular-nums">
      <div><dt>Logs with a trace</dt><dd>{(quality.logs - quality.uncorrelated_logs).toLocaleString()} of {quality.logs.toLocaleString()}</dd></div>
      <div><dt>Spans with a parent</dt><dd>{(quality.spans - quality.orphan_spans).toLocaleString()} of {quality.spans.toLocaleString()}</dd>
        <div className="mini-track mt-1.25 h-1.5 overflow-hidden rounded-full bg-glass-sunken" aria-hidden="true"><div className="mini-fill h-full rounded-[inherit] bg-linear-to-r from-sky-600 to-sky-400" style={{ width: `${linked * 100}%` }} /></div></div>
    </dl>
  </div>;
}
