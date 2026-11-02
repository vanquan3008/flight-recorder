/** A tiny trend line for a stat card. Decorative: the card already states the current value in text. */
export function Sparkline({ values, tone = "var(--accent)" }: { values: number[]; tone?: string }) {
  if (values.length < 2) return null;
  const width = 100;
  const height = 28;
  const pad = 3;
  const maximum = Math.max(...values);
  const minimum = Math.min(...values);
  const span = maximum - minimum;
  const x = (index: number) => (index / (values.length - 1)) * width;
  // A flat series sits on the baseline instead of dividing by zero.
  const y = (value: number) => span === 0 ? height - pad : height - pad - ((value - minimum) / span) * (height - pad * 2);
  const line = values.map((value, index) => `${index === 0 ? "M" : "L"} ${x(index).toFixed(2)} ${y(value).toFixed(2)}`).join(" ");
  const area = `${line} L ${width} ${height} L 0 ${height} Z`;
  const id = `spark-${tone.replace(/[^a-z0-9]/gi, "")}`;
  return (
    <svg className="sparkline" viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none" aria-hidden="true" focusable="false">
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={tone} stopOpacity=".32" />
          <stop offset="100%" stopColor={tone} stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={area} fill={`url(#${id})`} />
      <path d={line} fill="none" stroke={tone} strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
    </svg>
  );
}
