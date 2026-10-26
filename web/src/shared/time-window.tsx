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

export function TimeWindowFilter({ range, select, allowAny = false }: { range: string; select: (value: string) => void; allowAny?: boolean }) {
  return <div className="toolbar">
    <label>Time range <select aria-label="Time range" value={range} onChange={(event) => select(event.target.value)}>
      <option value="15m">Last 15 minutes</option><option value="1h">Last hour</option>
      <option value="24h">Last 24 hours</option><option value="7d">Last 7 days</option>
      {allowAny && <option value="any">Any time</option>}
    </select></label>
    <button type="button" onClick={() => select(range)}>Refresh window</button>
  </div>;
}
