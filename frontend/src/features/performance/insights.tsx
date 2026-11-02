import { queryString } from "../../shared/api";
import type { PerformanceGroup, TelemetryQuality } from "../../shared/types";
import { DetailLink, Empty, formatDuration } from "../../shared/ui";

const topServices = 6;
const percent = (value: number) => `${(value * 100).toFixed(value > 0 && value < 0.001 ? 2 : 1)}%`;

// Services ranked by request errors. The bar is relative to the worst service in the window.
export function ServiceErrorBars({ services, from, to }: { services: PerformanceGroup[]; from: string; to: string }) {
  if (!services.length) return <Empty>No inbound requests in this window. Check server instrumentation and the selected filters.</Empty>;
  const ranked = [...services].sort((a, b) => b.errors - a.errors || b.requests - a.requests).slice(0, topServices);
  const worst = Math.max(...ranked.map((service) => service.errors));
  return <>
    <ul className="service-bars">{ranked.map((service) => <li key={service.service}>
      <DetailLink to={`/traces?${queryString({ service: service.service, from, to })}`}>{service.service}</DetailLink>
      <div className="bar-track" role="img" aria-label={`${service.service}: ${service.errors} errors of ${service.requests} requests`}>
        {service.errors > 0 && <div className="bar-fill" style={{ width: `${Math.max(3, (service.errors / worst) * 100)}%` }} />}
      </div>
      <span className="bar-meta"><span><strong>{service.errors.toLocaleString()}</strong> errors · {percent(service.error_rate)}</span>
        <small>{service.requests.toLocaleString()} req · p95 {service.requests ? formatDuration(service.p95_latency_nanos) : "—"}</small></span>
    </li>)}</ul>
    {worst === 0 && <p className="insight-note">No request errors in this window.</p>}
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
  return <div className="quality">
    <svg className="donut" viewBox="0 0 100 100" role="img" aria-label={quality.logs ? `${label} of logs carry a trace` : "No logs in this window"}>
      <circle className="donut-track" cx="50" cy="50" r={radius} fill="none" strokeWidth="11" />
      {quality.logs > 0 && <circle className="donut-fill" cx="50" cy="50" r={radius} fill="none" strokeWidth="11" strokeLinecap="round"
        strokeDasharray={`${circumference * correlated} ${circumference}`} transform="rotate(-90 50 50)" />}
      <text className="donut-value" x="50" y="53" textAnchor="middle">{label}</text>
      <text className="donut-caption" x="50" y="67" textAnchor="middle">with trace</text>
    </svg>
    <dl className="quality-rows">
      <div><dt>Logs with a trace</dt><dd>{(quality.logs - quality.uncorrelated_logs).toLocaleString()} of {quality.logs.toLocaleString()}</dd></div>
      <div><dt>Spans with a parent</dt><dd>{(quality.spans - quality.orphan_spans).toLocaleString()} of {quality.spans.toLocaleString()}</dd>
        <div className="mini-track" aria-hidden="true"><div className="mini-fill" style={{ width: `${linked * 100}%` }} /></div></div>
    </dl>
  </div>;
}
