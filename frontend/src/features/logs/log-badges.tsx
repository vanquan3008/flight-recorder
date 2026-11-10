const tones = {
  blue: "border-sky-500/20 bg-sky-500/8 text-sky-700 dark:text-sky-300",
  green: "border-emerald-500/20 bg-emerald-500/8 text-emerald-700 dark:text-emerald-300",
  amber: "border-amber-500/20 bg-amber-500/8 text-amber-700 dark:text-amber-300",
  red: "border-rose-500/20 bg-rose-500/8 text-rose-700 dark:text-rose-300",
  neutral: "border-line bg-neutral-bg text-muted",
};
const badgeClass = "inline-flex h-5 min-w-0 items-center justify-center rounded border px-1.5 font-mono text-[10px] font-semibold leading-none tabular-nums";

export function MethodBadge({ method }: { method: string }) {
  const upper = method.toUpperCase();
  const tone = upper === "GET" ? tones.blue : upper === "POST" ? tones.green : ["PUT", "PATCH"].includes(upper) ? tones.amber : upper === "DELETE" ? tones.red : tones.neutral;
  return <span className={`method-badge ${badgeClass} ${tone}`}>{upper}</span>;
}

export function HttpStatusBadge({ status }: { status: number }) {
  const tone = status >= 500 ? tones.red : status >= 400 ? tones.amber : status >= 300 ? tones.blue : tones.green;
  return <span className={`status-badge ${badgeClass} ${tone}`}>{status}</span>;
}
