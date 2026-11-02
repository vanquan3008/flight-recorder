import { RefreshCw } from "lucide-react";
import { useMemo } from "react";
import { useSearchParams } from "react-router-dom";

const durations = { "15m": 15 * 60_000, "1h": 60 * 60_000, "24h": 24 * 60 * 60_000, "7d": 7 * 24 * 60 * 60_000 };
type Range = keyof typeof durations;

export function useTimeWindow() {
  const [params, setParams] = useSearchParams();
  const selected = params.get("range") || "15m";
  const range: Range = Object.hasOwn(durations, selected) ? selected as Range : "15m";
  const anchor = params.get("to");
  const window = useMemo(() => {
    const parsed = anchor ? Date.parse(anchor) : NaN;
    const end = Number.isFinite(parsed) ? parsed : Date.now();
    return { from: new Date(end - durations[range]).toISOString(), to: new Date(end).toISOString() };
  }, [range, anchor]);
  const select = (value: string) => {
    const next = new URLSearchParams(params);
    next.set("range", value);
    next.set("to", new Date().toISOString());
    setParams(next);
  };
  return { ...window, range, select };
}

const choices = [["15m", "15m", "Last 15 minutes"], ["1h", "1h", "Last hour"], ["24h", "24h", "Last 24 hours"], ["7d", "7d", "Last 7 days"]] as const;

export function TimeWindowFilter({ range, select, allowAny = false }: { range: string; select: (value: string) => void; allowAny?: boolean }) {
  const shown = allowAny ? [...choices, ["any", "Any", "Any time"] as const] : choices;
  return (
    <div className="toolbar time-filter">
      <span className="time-filter-label" aria-hidden="true">Time range</span>
      <fieldset className="segmented" aria-label="Time range">
        {shown.map(([value, text, name]) => (
          <label className="segment" key={value}>
            <input type="radio" name="time-range" value={value} checked={range === value} aria-label={name} onChange={() => select(value)} />
            <span>{text}</span>
          </label>
        ))}
      </fieldset>
      <button type="button" className="refresh" aria-label="Refresh window" onClick={() => select(range)}>
        <RefreshCw size={13} aria-hidden="true" /> Refresh
      </button>
    </div>
  );
}
