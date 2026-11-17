import { buildSmoothSpline } from "./spline";
import { useRef, useState } from "react";
import type { PerformanceBucket } from "../../shared/types";
import { formatDuration, formatTime } from "../../shared/ui";

const pad = (n: number) => String(n).padStart(2, "0");

export const formatAxisTick = (isoString?: string, durationMs = 0) => {
  if (!isoString) return "";
  const t = Date.parse(isoString);
  if (Number.isNaN(t)) return "";
  const d = new Date(t);
  const time = `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  if (durationMs > 24 * 3600_000) {
    return `${pad(d.getDate())}/${pad(d.getMonth() + 1)} ${time}`;
  }
  return time;
};

export const formatTooltipTime = (fromIso?: string, toIso?: string) => {
  if (!fromIso) return "";
  const d1 = new Date(fromIso);
  if (Number.isNaN(d1.getTime())) return "";
  const y = d1.getFullYear();
  const m = pad(d1.getMonth() + 1);
  const d = pad(d1.getDate());
  const t1 = `${pad(d1.getHours())}:${pad(d1.getMinutes())}`;

  if (!toIso) return `${y}-${m}-${d} ${t1}`;
  const d2 = new Date(toIso);
  if (Number.isNaN(d2.getTime())) return `${y}-${m}-${d} ${t1}`;
  const t2 = `${pad(d2.getHours())}:${pad(d2.getMinutes())}`;

  const isSameDay = d1.getFullYear() === d2.getFullYear() && d1.getMonth() === d2.getMonth() && d1.getDate() === d2.getDate();
  if (isSameDay) {
    return `${y}-${m}-${d} ${t1} — ${t2}`;
  }
  const m2 = pad(d2.getMonth() + 1);
  const d2Date = pad(d2.getDate());
  return `${m}/${d} ${t1} — ${m2}/${d2Date} ${t2}`;
};

const formatDateRange = (from?: string, to?: string) => {
  if (!from || !to) return "";
  const d1 = new Date(from);
  const d2 = new Date(to);
  if (Number.isNaN(d1.getTime()) || Number.isNaN(d2.getTime())) return "";
  const f1 = `${d1.getFullYear()}-${pad(d1.getMonth() + 1)}-${pad(d1.getDate())}`;
  const f2 = `${d2.getFullYear()}-${pad(d2.getMonth() + 1)}-${pad(d2.getDate())}`;
  return f1 === f2 ? f1 : `${f1} — ${f2}`;
};

export function PerformanceTrend({ series, kind }: { series: PerformanceBucket[]; kind: "rate" | "latency" }) {
  const rate = kind === "rate";
  const [hoverIndex, setHoverIndex] = useState<number | null>(null);
  const svgRef = useRef<SVGSVGElement>(null);

  const lines = rate
    ? [{ name: "Requests/s", color: "#0284c7", value: (b: PerformanceBucket) => b.requests_per_second },
       { name: "Errors/s", color: "#ef4444", value: (b: PerformanceBucket) => b.errors / ((Date.parse(b.to) - Date.parse(b.from)) / 1000) }]
    : [{ name: "P95", color: "#0284c7", value: (b: PerformanceBucket) => b.p95_latency_nanos },
       { name: "P99", color: "#f59e0b", value: (b: PerformanceBucket) => b.p99_latency_nanos }];

  const hasRequests = series.some((bucket) => bucket.requests > 0);
  const maximum = Math.max(rate ? 0.1 : 1e6, ...lines.flatMap((line) => series.map(line.value)));
  const start = series[0]?.from ? Date.parse(series[0].from) : 0;
  const end = series.at(-1)?.to ? Date.parse(series.at(-1)!.to) : 0;
  const validWindow = Number.isFinite(start) && Number.isFinite(end) && end > start;
  const duration = validWindow ? end - start : 1;
  const midTime = validWindow ? new Date(start + duration / 2).toISOString() : "";

  const x = (bucket: PerformanceBucket) => 70 + 550 * ((Date.parse(bucket.from) + Date.parse(bucket.to)) / 2 - start) / duration;
  const y = (value: number) => 135 - 110 * (Number.isFinite(value) ? value : 0) / maximum;
  const label = (value: number) => rate ? value.toLocaleString("en", { maximumSignificantDigits: 3 }) : value === 0 ? "0ms" : formatDuration(value);

  const validBuckets = series.filter((b) => rate || b.requests > 0);
  const primaryLine = lines[0];
  const secondaryLine = lines[1];

  const primaryValues = validBuckets.map(primaryLine.value);
  const peakPrimary = primaryValues.length > 0 ? Math.max(0, ...primaryValues) : 0;
  const totalErrors = rate ? series.reduce((acc, b) => acc + b.errors, 0) : 0;

  const primaryPoints = validBuckets.map((b) => ({ x: x(b), y: y(primaryLine.value(b)) }));
  const secondaryPoints = validBuckets.map((b) => ({ x: x(b), y: y(secondaryLine.value(b)) }));

  const primarySpline = buildSmoothSpline(primaryPoints, 135);
  const secondarySpline = buildSmoothSpline(secondaryPoints, 135);
  const hasSecondaryValues = validBuckets.some((b) => secondaryLine.value(b) > 0);

  const handleMouseMove = (event: React.MouseEvent<SVGSVGElement>) => {
    if (!svgRef.current || validBuckets.length === 0) return;
    const rect = svgRef.current.getBoundingClientRect();
    const svgX = ((event.clientX - rect.left) / rect.width) * 640;

    let closestIdx = 0;
    let minDiff = Infinity;
    validBuckets.forEach((bucket, idx) => {
      const diff = Math.abs(x(bucket) - svgX);
      if (diff < minDiff) {
        minDiff = diff;
        closestIdx = idx;
      }
    });
    setHoverIndex(closestIdx);
  };

  const handleMouseLeave = () => {
    setHoverIndex(null);
  };

  const hoveredBucket = hoverIndex !== null ? validBuckets[hoverIndex] : null;
  const hoverX = hoveredBucket ? x(hoveredBucket) : 0;
  const tooltipX = hoverX > 400 ? hoverX - 195 : hoverX + 14;

  return <figure className="performance-trend m-0 min-w-0 flex flex-col justify-between h-full rounded-xl border border-glass-divider bg-surface/35 px-4 pt-3 pb-2.5 [&>figcaption]:flex [&>figcaption]:flex-wrap [&>figcaption]:items-start [&>figcaption]:justify-between [&>figcaption]:gap-2 [&>figcaption]:text-[12.5px] [&>figcaption]:font-semibold [&>svg]:mt-1.5 [&>svg]:block [&>svg]:h-auto [&>svg]:w-full [&>svg]:cursor-crosshair [&>svg]:overflow-visible [&_svg_text]:fill-faint [&_svg_text]:font-mono [&_svg_text]:text-[10.5px]">
    <figcaption>
      <div className="performance-title-group flex flex-col gap-0.5">
        <div className="performance-title-row flex flex-wrap items-center gap-2">
          <strong>{rate ? "Observed request rate" : "Observed request latency"}</strong>
          {hasRequests && (
            <div className="performance-hud-chips inline-flex items-center gap-1">
              <span className="hud-chip inline-flex items-center gap-1 rounded-full border border-glass-divider bg-glass-field px-1.5 py-px text-[9.5px] leading-snug">
                <span className="hud-chip-label text-[9px] font-bold tracking-wider text-muted">PEAK</span>
                <span className="hud-chip-value font-mono font-bold text-text">{label(peakPrimary)}</span>
              </span>
              {rate && (
                <span className={`hud-chip inline-flex items-center gap-1 rounded-full border border-glass-divider bg-glass-field px-1.5 py-px text-[9.5px] leading-snug ${totalErrors > 0 ? "hud-chip-warn border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-400" : "hud-chip-ok border-emerald-500/30 bg-emerald-500/8 text-emerald-600 dark:text-emerald-400"}`}>
                  <i className="hud-dot size-1.25 rounded-full bg-current" />
                  <span className="hud-chip-label text-[9px] font-bold tracking-wider text-muted">{totalErrors > 0 ? `${totalErrors} ERR` : "STABLE"}</span>
                </span>
              )}
            </div>
          )}
        </div>
        {series.length > 0 && (
          <div className="flex items-center gap-2">
            <span className="performance-chart-date font-mono text-[11px] font-semibold text-accent">
              {formatDateRange(series[0]?.from, series.at(-1)?.to)}
            </span>
            <span className="text-[10px] font-semibold tracking-wide uppercase text-muted rounded-md bg-neutral-bg px-1.5 py-0.5">
              {duration > 24 * 3600_000 ? `${Math.round(duration / 86400000)}d range` : duration > 3600_000 ? `${Math.round(duration / 3600000)}h range` : `${Math.round(duration / 60000)}m range`}
            </span>
          </div>
        )}
      </div>
      <span className="performance-legend flex gap-3 text-[11px] font-medium text-muted [&>span]:inline-flex [&>span]:items-center [&>span]:gap-1.25 [&_i]:size-1.75 [&_i]:rounded-full">{lines.map((line) =>
        <span key={line.name}><i style={{ background: line.color }} />{line.name}</span>)}</span>
    </figcaption>
    {hasRequests ? <svg
      ref={svgRef}
      viewBox="0 0 640 165"
      role="img"
      aria-label={rate ? "Observed request rate over time" : "Observed request latency over time"}
      onMouseMove={handleMouseMove}
      onMouseLeave={handleMouseLeave}
    >
      <defs>
        {/* Clean, subtle gradient fill */}
        <linearGradient id={`area-grad-${kind}`} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="#0284c7" stopOpacity="0.16" />
          <stop offset="65%" stopColor="#0284c7" stopOpacity="0.03" />
          <stop offset="100%" stopColor="#0284c7" stopOpacity="0.0" />
        </linearGradient>
      </defs>
      <title>{rate ? "Request and error rates, including idle intervals" : "P95 and P99 of received requests; gaps have no samples"}</title>

      {/* Grid lines */}
      {[0, 0.5, 1].map((fraction) => {
        const lineY = y(maximum * fraction);
        return (
          <g key={fraction} className="chart-grid-line">
            <line x1="70" x2="620" y1={lineY} y2={lineY} stroke="var(--line)" strokeDasharray="3 3" opacity="0.35" />
            <text x="62" y={lineY + 4} textAnchor="end">{label(maximum * fraction)}</text>
          </g>
        );
      })}

      {/* Area under primary curve */}
      {primarySpline.areaPath && (
        <path d={primarySpline.areaPath} fill={`url(#area-grad-${kind})`} />
      )}

      {/* Secondary line (Errors/s or P99) */}
      {secondarySpline.linePath && (
        <path
          d={secondarySpline.linePath}
          fill="none"
          stroke={hasSecondaryValues ? secondaryLine.color : "var(--glass-divider)"}
          strokeWidth={hasSecondaryValues ? "2" : "1"}
          strokeOpacity={hasSecondaryValues ? 0.95 : 0.25}
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      )}

      {/* Primary line (Requests/s or P95) */}
      {primarySpline.linePath && (
        <path
          d={primarySpline.linePath}
          fill="none"
          stroke={primaryLine.color}
          strokeWidth="2.2"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      )}

      {/* Peak points on primary line */}
      {validBuckets.map((bucket) => {
        const val = primaryLine.value(bucket);
        if (val <= 0) return null;
        const cx = x(bucket);
        const cy = y(val);
        return (
          <g key={`pri-${bucket.from}`}>
            <circle cx={cx} cy={cy} r="3.5" fill="var(--surface-solid)" stroke={primaryLine.color} strokeWidth="1.8" />
            <title>{`${formatTime(bucket.from)} – ${formatTime(bucket.to)}: ${primaryLine.name} ${label(val)}`}</title>
          </g>
        );
      })}

      {/* Peak points on secondary line */}
      {hasSecondaryValues && validBuckets.map((bucket) => {
        const val = secondaryLine.value(bucket);
        if (val <= 0) return null;
        const cx = x(bucket);
        const cy = y(val);
        return (
          <g key={`sec-${bucket.from}`}>
            <circle cx={cx} cy={cy} r="3" fill="var(--surface-solid)" stroke={secondaryLine.color} strokeWidth="1.6" />
            <title>{`${formatTime(bucket.from)} – ${formatTime(bucket.to)}: ${secondaryLine.name} ${label(val)}`}</title>
          </g>
        );
      })}

      {/* Interactive hover crosshair and tooltip */}
      {hoveredBucket && (
        <g className="chart-interactive-hover transition-opacity pointer-events-none select-none">
          {/* Vertical guideline */}
          <line x1={hoverX} x2={hoverX} y1={15} y2={135} stroke="var(--line-strong)" strokeWidth="1" strokeDasharray="3 3" opacity="0.6" />

          {/* Active hover dot on primary */}
          <circle cx={hoverX} cy={y(primaryLine.value(hoveredBucket))} r="4.5" fill={primaryLine.color} stroke="#ffffff" strokeWidth="2" />

          {/* Active hover dot on secondary */}
          {hasSecondaryValues && (
            <circle cx={hoverX} cy={y(secondaryLine.value(hoveredBucket))} r="4" fill={secondaryLine.color} stroke="#ffffff" strokeWidth="1.8" />
          )}

          {/* Floating glass tooltip */}
          <g transform={`translate(${tooltipX}, 14)`}>
            <rect
              width="185"
              height="68"
              rx="10"
              ry="10"
              fill="var(--surface-header)"
              stroke="var(--glass-edge)"
              strokeWidth="1"
              filter="drop-shadow(0 8px 20px rgba(0,0,0,0.25))"
            />
            <text x="12" y="17" fill="var(--accent)" fontSize="10.5" fontFamily="var(--font-mono)" fontWeight="700">
              {formatTooltipTime(hoveredBucket.from, hoveredBucket.to)}
            </text>
            <circle cx="16" cy="34" r="3.5" fill={primaryLine.color} />
            <text x="25" y="37" fill="var(--text)" fontSize="11" fontWeight="600">
              {primaryLine.name}: <tspan fontFamily="var(--font-mono)" fontWeight="700">{label(primaryLine.value(hoveredBucket))}</tspan>
            </text>
            <circle cx="16" cy="51" r="3.5" fill={secondaryLine.color} />
            <text x="25" y="54" fill="var(--text)" fontSize="11" fontWeight="600">
              {secondaryLine.name}: <tspan fontFamily="var(--font-mono)" fontWeight="700" fill={secondaryLine.value(hoveredBucket) > 0 ? "var(--danger)" : "var(--muted)"}>{label(secondaryLine.value(hoveredBucket))}</tspan>
            </text>
          </g>
        </g>
      )}

      {/* X Axis */}
      <g className="chart-x-axis [&_text]:fill-muted [&_text]:text-[10px] [&_text]:font-mono [&_text]:font-semibold" textAnchor="middle">
        <text x="70" y="156" textAnchor="start">{formatAxisTick(series[0]?.from, duration)}</text>
        {midTime && <text x="345" y="156">{formatAxisTick(midTime, duration)}</text>}
        <text x="620" y="156" textAnchor="end">{formatAxisTick(series.at(-1)?.to, duration)}</text>
      </g>
    </svg> : <p className="empty m-0 p-8 text-center text-sm text-muted">No inbound requests to plot.</p>}
  </figure>;
}
