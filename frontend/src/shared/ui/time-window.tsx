import { RefreshCw } from "lucide-react";
import { useMemo } from "react";
import { useSearchParams } from "react-router-dom";

const durations = { "15m": 15 * 60_000, "1h": 60 * 60_000, "24h": 24 * 60 * 60_000, "7d": 7 * 24 * 60 * 60_000 };
type Range = keyof typeof durations;

export function useTimeWindow() {
  const [params, setParams] = useSearchParams();
  const selected = params.get("range") || "15m";
  const preset: Range = Object.hasOwn(durations, selected) ? selected as Range : "15m";
  const anchor = params.get("to");
  const start = params.get("from");
  const window = useMemo(() => {
    const parsed = anchor ? Date.parse(anchor) : NaN;
    const parsedStart = start ? Date.parse(start) : NaN;
    const end = Number.isFinite(parsed) ? parsed : Date.now();
    const explicit = Number.isFinite(parsed) && Number.isFinite(parsedStart) && parsedStart < end;
    const duration = explicit ? end - parsedStart : durations[preset];
    const range = explicit ? Object.entries(durations).find(([, value]) => value === duration)?.[0] ?? "custom" : preset;
    return { from: new Date(end - duration).toISOString(), to: new Date(end).toISOString(), range, duration };
  }, [preset, anchor, start]);
  const select = (value: string) => {
    const next = new URLSearchParams(params);
    const end = Date.now();
    next.delete("from");
    next.delete("cursor");
    next.set("range", value === "custom" ? "15m" : value);
    next.set("to", new Date(end).toISOString());
    if (value === "custom") next.set("from", new Date(end - window.duration).toISOString());
    setParams(next);
  };
  return { ...window, select };
}

const choices = [["15m", "15m", "Last 15 minutes"], ["1h", "1h", "Last hour"], ["24h", "24h", "Last 24 hours"], ["7d", "7d", "Last 7 days"]] as const;

export function TimeWindowFilter({ range, select, allowAny = false, from, to }: { range: string; select: (value: string) => void; allowAny?: boolean; from?: string; to?: string }) {
  const shown = allowAny ? [...choices, ["any", "Any", "Any time"] as const] : choices;
  return (
    <div className="time-filter flex flex-wrap items-center gap-2.5" title={from && to && range !== "any" ? `${new Date(from).toLocaleString()} – ${new Date(to).toLocaleString()} (${Intl.DateTimeFormat().resolvedOptions().timeZone})` : undefined}>
      <span className="time-filter-label sr-only" aria-hidden="true">Time range</span>
      <fieldset className="segmented inline-flex min-w-0 gap-0.5 rounded-lg border border-glass-field-edge bg-glass-field p-0.75 m-0 " aria-label="Time range">
        {range === "custom" && <span className="segment relative inline-flex cursor-pointer" aria-label="Custom time range">Custom</span>}
        {shown.map(([value, text, name]) => (
          <label className="segment relative inline-flex cursor-pointer" key={value}>
            <input className="peer absolute inset-0 size-full cursor-pointer opacity-0" type="radio" name="time-range" value={value} checked={range === value} aria-label={name} onChange={() => select(value)} />
            <span className="rounded-md px-3 py-1 text-xs font-semibold leading-snug text-muted peer-checked:bg-btn peer-checked:text-btn-text peer-checked:shadow-sm peer-focus-visible:outline-2 peer-focus-visible:outline-accent">{text}</span>
          </label>
        ))}
      </fieldset>
      <button type="button" className="refresh inline-flex items-center gap-1.5 rounded-lg border border-glass-field-edge bg-glass-field px-3 py-1.25 text-xs font-semibold hover:bg-glass-field-focus" aria-label="Refresh window" onClick={() => select(range)}>
        <RefreshCw size={13} aria-hidden="true" /> Refresh
      </button>
    </div>
  );
}
