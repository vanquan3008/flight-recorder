import type { PerformanceBucket } from "../../shared/types";
import { formatDuration, formatTime } from "../../shared/ui";

export function PerformanceTrend({ series, kind }: { series: PerformanceBucket[]; kind: "rate" | "latency" }) {
  const rate = kind === "rate";
  const lines = rate
    ? [{ name: "Requests/s", color: "var(--accent)", value: (bucket: PerformanceBucket) => bucket.requests_per_second },
      { name: "Errors/s", color: "var(--danger)", value: (bucket: PerformanceBucket) => bucket.errors / ((Date.parse(bucket.to) - Date.parse(bucket.from)) / 1000) }]
    : [{ name: "P95", color: "var(--accent-blue)", value: (bucket: PerformanceBucket) => bucket.p95_latency_nanos },
      { name: "P99", color: "var(--warning)", value: (bucket: PerformanceBucket) => bucket.p99_latency_nanos }];
  const hasRequests = series.some((bucket) => bucket.requests > 0);
  const maximum = Math.max(rate ? 0.1 : 1e6, ...lines.flatMap((line) => series.map(line.value)));
  const start = Date.parse(series[0]?.from);
  const duration = Date.parse(series.at(-1)?.to || "") - start;
  const x = (bucket: PerformanceBucket) => 70 + 550 * ((Date.parse(bucket.from) + Date.parse(bucket.to)) / 2 - start) / duration;
  const y = (value: number) => 175 - 150 * value / maximum;
  const label = (value: number) => rate ? value.toLocaleString("en", { maximumSignificantDigits: 3 }) : value === 0 ? "0ms" : formatDuration(value);
  return <figure className="performance-trend">
    <figcaption><strong>{rate ? "Observed request rate" : "Observed request latency"}</strong><span className="performance-legend">{lines.map((line) =>
      <span key={line.name}><i style={{ background: line.color }} />{line.name}</span>)}</span></figcaption>
    {hasRequests ? <svg viewBox="0 0 640 215" role="img" aria-label={rate ? "Observed request rate over time" : "Observed request latency over time"}>
      <title>{rate ? "Request and error rates, including idle intervals" : "P95 and P99 of received requests; gaps have no samples"}</title>
      {[0, 0.5, 1].map((fraction) => <g key={fraction}>
        <line x1="70" x2="620" y1={y(maximum * fraction)} y2={y(maximum * fraction)} stroke="var(--line)" />
        <text x="62" y={y(maximum * fraction) + 4} textAnchor="end">{label(maximum * fraction)}</text>
      </g>)}
      {lines.map((line) => <g key={line.name} stroke={line.color} fill={line.color}>
        {series.map((bucket, index) => {
          if (!rate && !bucket.requests) return null;
          const previous = series[index - 1];
          return <g key={bucket.from}>
            {previous && (rate || previous.requests > 0) && <line x1={x(previous)} x2={x(bucket)} y1={y(line.value(previous))} y2={y(line.value(bucket))} strokeWidth="2" />}
            <circle cx={x(bucket)} cy={y(line.value(bucket))} r="2.5"><title>{`${formatTime(bucket.from)} – ${formatTime(bucket.to)}: ${line.name} ${label(line.value(bucket))}`}</title></circle>
          </g>;
        })}
      </g>)}
      <text x="70" y="205">Start of selected window</text><text x="620" y="205" textAnchor="end">End of selected window</text>
    </svg> : <p className="empty">No inbound requests to plot.</p>}
    <p className="performance-chart-window">{formatTime(series[0]?.from)} — {formatTime(series.at(-1)?.to)}</p>
  </figure>;
}
