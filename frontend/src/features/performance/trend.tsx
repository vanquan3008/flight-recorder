import { useRef, useState } from "react";
import type { PerformanceBucket } from "../../shared/types";
import { formatDuration, formatTime } from "../../shared/ui";
import "./performance.css";

const formatClock = (isoString?: string) => {
  if (!isoString) return "";
  const t = Date.parse(isoString);
  return Number.isNaN(t) ? "" : new Date(t).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
};

function buildSmoothSpline(pts: { x: number; y: number }[], baselineY = 175): { linePath: string; areaPath: string } {
  const n = pts.length;
  if (n === 0) return { linePath: "", areaPath: "" };
  if (n === 1) {
    const x0 = pts[0].x.toFixed(1);
    const y0 = pts[0].y.toFixed(1);
    return {
      linePath: `M ${x0} ${y0}`,
      areaPath: `M ${x0} ${baselineY} L ${x0} ${y0} L ${x0} ${baselineY} Z`,
    };
  }

  const dx: number[] = [];
  const dy: number[] = [];
  const slope: number[] = [];
  for (let i = 0; i < n - 1; i++) {
    dx[i] = pts[i + 1].x - pts[i].x;
    dy[i] = pts[i + 1].y - pts[i].y;
    slope[i] = dx[i] !== 0 ? dy[i] / dx[i] : 0;
  }

  const m: number[] = new Array(n).fill(0);
  m[0] = slope[0];
  m[n - 1] = slope[n - 2];
  for (let i = 1; i < n - 1; i++) {
    if (slope[i - 1] * slope[i] <= 0) {
      m[i] = 0;
    } else {
      m[i] = (slope[i - 1] + slope[i]) / 2;
    }
  }

  let curvePart = "";
  for (let i = 0; i < n - 1; i++) {
    const p1 = pts[i];
    const p2 = pts[i + 1];
    const segDx = dx[i] / 3;
    const cp1x = p1.x + segDx;
    const cp1y = Math.min(baselineY, Math.max(15, p1.y + m[i] * segDx));
    const cp2x = p2.x - segDx;
    const cp2y = Math.min(baselineY, Math.max(15, p2.y - m[i + 1] * segDx));

    curvePart += ` C ${cp1x.toFixed(1)} ${cp1y.toFixed(1)}, ${cp2x.toFixed(1)} ${cp2y.toFixed(1)}, ${p2.x.toFixed(1)} ${p2.y.toFixed(1)}`;
  }

  const startX = pts[0].x.toFixed(1);
  const startY = pts[0].y.toFixed(1);
  const endX = pts[n - 1].x.toFixed(1);

  const linePath = `M ${startX} ${startY}${curvePart}`;
  const areaPath = `M ${startX} ${baselineY} L ${startX} ${startY}${curvePart} L ${endX} ${baselineY} Z`;

  return { linePath, areaPath };
}

export function PerformanceTrend({ series, kind }: { series: PerformanceBucket[]; kind: "rate" | "latency" }) {
  const rate = kind === "rate";
  const [hoverIndex, setHoverIndex] = useState<number | null>(null);
  const svgRef = useRef<SVGSVGElement>(null);

  const lines = rate
    ? [{ name: "Requests/s", color: "var(--accent)", value: (b: PerformanceBucket) => b.requests_per_second },
       { name: "Errors/s", color: "var(--danger)", value: (b: PerformanceBucket) => b.errors / ((Date.parse(b.to) - Date.parse(b.from)) / 1000) }]
    : [{ name: "P95", color: "var(--accent)", value: (b: PerformanceBucket) => b.p95_latency_nanos },
       { name: "P99", color: "var(--warning)", value: (b: PerformanceBucket) => b.p99_latency_nanos }];

  const hasRequests = series.some((bucket) => bucket.requests > 0);
  const maximum = Math.max(rate ? 0.1 : 1e6, ...lines.flatMap((line) => series.map(line.value)));
  const start = series[0]?.from ? Date.parse(series[0].from) : 0;
  const end = series.at(-1)?.to ? Date.parse(series.at(-1)!.to) : 0;
  const validWindow = Number.isFinite(start) && Number.isFinite(end) && end > start;
  const duration = validWindow ? end - start : 1;
  const midTime = validWindow ? new Date(start + duration / 2).toISOString() : "";

  const x = (bucket: PerformanceBucket) => 70 + 550 * ((Date.parse(bucket.from) + Date.parse(bucket.to)) / 2 - start) / duration;
  const y = (value: number) => 175 - 150 * (Number.isFinite(value) ? value : 0) / maximum;
  const label = (value: number) => rate ? value.toLocaleString("en", { maximumSignificantDigits: 3 }) : value === 0 ? "0ms" : formatDuration(value);

  const validBuckets = series.filter((b) => rate || b.requests > 0);
  const primaryLine = lines[0];
  const secondaryLine = lines[1];

  const primaryValues = validBuckets.map(primaryLine.value);
  const peakPrimary = primaryValues.length > 0 ? Math.max(0, ...primaryValues) : 0;
  const totalErrors = rate ? series.reduce((acc, b) => acc + b.errors, 0) : 0;

  const primaryPoints = validBuckets.map((b) => ({ x: x(b), y: y(primaryLine.value(b)) }));
  const secondaryPoints = validBuckets.map((b) => ({ x: x(b), y: y(secondaryLine.value(b)) }));

  const primarySpline = buildSmoothSpline(primaryPoints, 175);
  const secondarySpline = buildSmoothSpline(secondaryPoints, 175);
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
  const tooltipX = hoverX > 430 ? hoverX - 165 : hoverX + 14;

  return <figure className="performance-trend">
    <figcaption>
      <div className="performance-title-group">
        <div className="performance-title-row">
          <strong>{rate ? "Observed request rate" : "Observed request latency"}</strong>
          {hasRequests && (
            <div className="performance-hud-chips">
              <span className="hud-chip">
                <span className="hud-chip-label">PEAK</span>
                <span className="hud-chip-value">{label(peakPrimary)}</span>
              </span>
              {rate && (
                <span className={`hud-chip ${totalErrors > 0 ? "hud-chip-warn" : "hud-chip-ok"}`}>
                  <i className="hud-dot" />
                  <span className="hud-chip-label">{totalErrors > 0 ? `${totalErrors} ERR` : "STABLE"}</span>
                </span>
              )}
            </div>
          )}
        </div>
        {series.length > 0 && (
          <span className="performance-chart-date">{formatTime(series[0]?.from)} — {formatTime(series.at(-1)?.to)}</span>
        )}
      </div>
      <span className="performance-legend">{lines.map((line) =>
        <span key={line.name}><i style={{ background: line.color }} />{line.name}</span>)}</span>
    </figcaption>
    {hasRequests ? <svg
      ref={svgRef}
      viewBox="0 0 640 215"
      role="img"
      aria-label={rate ? "Observed request rate over time" : "Observed request latency over time"}
      onMouseMove={handleMouseMove}
      onMouseLeave={handleMouseLeave}
    >
      <defs>
        {/* Cyberpunk Neon Area Gradient */}
        <linearGradient id={`area-grad-${kind}`} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="#06b6d4" stopOpacity="0.45" />
          <stop offset="35%" stopColor="#3b82f6" stopOpacity="0.22" />
          <stop offset="70%" stopColor="#6366f1" stopOpacity="0.06" />
          <stop offset="100%" stopColor="#6366f1" stopOpacity="0.0" />
        </linearGradient>

        {/* Dynamic Stroke Gradient */}
        <linearGradient id={`stroke-grad-${kind}`} x1="0" y1="0" x2="1" y2="0">
          <stop offset="0%" stopColor="#06b6d4" />
          <stop offset="50%" stopColor="#3b82f6" />
          <stop offset="100%" stopColor="#8b5cf6" />
        </linearGradient>

        {/* Multi-stage Neon Bloom Filter */}
        <filter id={`line-glow-${kind}`} x="-20%" y="-30%" width="140%" height="160%">
          <feGaussianBlur in="SourceGraphic" stdDeviation="2" result="blur1" />
          <feGaussianBlur in="SourceGraphic" stdDeviation="5.5" result="blur2" />
          <feMerge>
            <feMergeNode in="blur2" />
            <feMergeNode in="blur1" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>

        <filter id={`danger-glow-${kind}`} x="-20%" y="-30%" width="140%" height="160%">
          <feDropShadow dx="0" dy="0" stdDeviation="3.5" floodColor="#f43f5e" floodOpacity="0.6" />
        </filter>

        {/* Laser scanline vertical gradient */}
        <linearGradient id={`scan-laser-${kind}`} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="#06b6d4" stopOpacity="0.0" />
          <stop offset="50%" stopColor="#06b6d4" stopOpacity="0.75" />
          <stop offset="100%" stopColor="#06b6d4" stopOpacity="0.1" />
        </linearGradient>
      </defs>
      <title>{rate ? "Request and error rates, including idle intervals" : "P95 and P99 of received requests; gaps have no samples"}</title>

      {/* Grid lines */}
      {[0, 0.5, 1].map((fraction) => {
        const lineY = y(maximum * fraction);
        return (
          <g key={fraction} className="chart-grid-line">
            <line x1="70" x2="620" y1={lineY} y2={lineY} stroke="var(--line)" strokeDasharray="3 3" opacity="0.45" />
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
          strokeWidth={hasSecondaryValues ? "2.5" : "1"}
          strokeOpacity={hasSecondaryValues ? 1 : 0.25}
          strokeLinecap="round"
          strokeLinejoin="round"
          filter={hasSecondaryValues ? `url(#danger-glow-${kind})` : undefined}
        />
      )}

      {/* Primary line (Requests/s or P95) */}
      {primarySpline.linePath && (
        <path
          d={primarySpline.linePath}
          fill="none"
          stroke={`url(#stroke-grad-${kind})`}
          strokeWidth="2.8"
          strokeLinecap="round"
          strokeLinejoin="round"
          filter={`url(#line-glow-${kind})`}
        />
      )}

      {/* Peak points on primary line */}
      {validBuckets.map((bucket) => {
        const val = primaryLine.value(bucket);
        if (val <= 0) return null;
        const cx = x(bucket);
        const cy = y(val);
        const isMaxPeak = val === peakPrimary && peakPrimary > 0;
        return (
          <g key={`pri-${bucket.from}`}>
            {isMaxPeak && (
              <circle
                cx={cx}
                cy={cy}
                r="8"
                fill="none"
                stroke="#06b6d4"
                strokeWidth="1.5"
                opacity="0.6"
                className="chart-radar-ping"
              />
            )}
            <circle cx={cx} cy={cy} r="4.5" fill="var(--surface-solid)" stroke="#06b6d4" strokeWidth="2" />
            <circle cx={cx} cy={cy} r="2" fill="#06b6d4" />
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
            <circle cx={cx} cy={cy} r="4" fill="var(--surface-solid)" stroke={secondaryLine.color} strokeWidth="1.8" />
            <circle cx={cx} cy={cy} r="1.8" fill={secondaryLine.color} />
            <title>{`${formatTime(bucket.from)} – ${formatTime(bucket.to)}: ${secondaryLine.name} ${label(val)}`}</title>
          </g>
        );
      })}

      {/* Interactive hover crosshair and tooltip */}
      {hoveredBucket && (
        <g className="chart-interactive-hover pointer-events-none select-none">
          {/* Vertical laser guideline */}
          <line x1={hoverX} x2={hoverX} y1={20} y2={175} stroke={`url(#scan-laser-${kind})`} strokeWidth="1.5" />
          <circle cx={hoverX} cy={20} r="2" fill="#06b6d4" />

          {/* Active hover dot on primary */}
          <circle cx={hoverX} cy={y(primaryLine.value(hoveredBucket))} r="8" fill="#06b6d4" opacity="0.25" />
          <circle cx={hoverX} cy={y(primaryLine.value(hoveredBucket))} r="5" fill="#06b6d4" stroke="#ffffff" strokeWidth="2" />

          {/* Active hover dot on secondary */}
          {hasSecondaryValues && (
            <circle cx={hoverX} cy={y(secondaryLine.value(hoveredBucket))} r="4" fill={secondaryLine.color} stroke="#ffffff" strokeWidth="1.8" />
          )}

          {/* Floating glass tooltip */}
          <g transform={`translate(${tooltipX}, 18)`}>
            <rect
              width="160"
              height="64"
              rx="10"
              ry="10"
              fill="var(--surface-header)"
              stroke="var(--glass-edge)"
              strokeWidth="1"
              filter="drop-shadow(0 10px 24px rgba(0,0,0,0.45))"
            />
            <text x="12" y="17" fill="var(--muted)" fontSize="10" fontFamily="var(--font-mono)" fontWeight="600">
              {formatClock(hoveredBucket.from)} — {formatClock(hoveredBucket.to)}
            </text>
            <circle cx="16" cy="33" r="3.5" fill="#06b6d4" />
            <text x="25" y="36" fill="var(--text)" fontSize="11" fontWeight="600">
              {primaryLine.name}: <tspan fontFamily="var(--font-mono)" fontWeight="700">{label(primaryLine.value(hoveredBucket))}</tspan>
            </text>
            <circle cx="16" cy="50" r="3.5" fill={secondaryLine.color} />
            <text x="25" y="53" fill="var(--text)" fontSize="11" fontWeight="600">
              {secondaryLine.name}: <tspan fontFamily="var(--font-mono)" fontWeight="700" fill={secondaryLine.value(hoveredBucket) > 0 ? "var(--danger)" : "var(--muted)"}>{label(secondaryLine.value(hoveredBucket))}</tspan>
            </text>
          </g>
        </g>
      )}

      {/* X Axis */}
      <g className="chart-x-axis" textAnchor="middle">
        <text x="70" y="202" textAnchor="start">{formatClock(series[0]?.from)}</text>
        {midTime && <text x="345" y="202">{formatClock(midTime)}</text>}
        <text x="620" y="202" textAnchor="end">{formatClock(series.at(-1)?.to)}</text>
      </g>
    </svg> : <p className="empty">No inbound requests to plot.</p>}
  </figure>;
}
