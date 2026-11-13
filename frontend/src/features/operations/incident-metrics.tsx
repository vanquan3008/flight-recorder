import { AlertCircle, CheckCircle2, Flame } from "lucide-react";

export function StatusBadge({ status }: { status: string }) {
  const norm = status.toUpperCase();
  if (norm === "OPEN") {
    return (
      <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-bold tracking-wider uppercase bg-rose-500/15 text-rose-600 dark:text-rose-400 border border-rose-500/30 shadow-2xs">
        <span className="size-1.5 rounded-full bg-rose-500 animate-pulse" aria-hidden="true" />
        OPEN
      </span>
    );
  }
  if (norm === "INVESTIGATING") {
    return (
      <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-bold tracking-wider uppercase bg-amber-500/15 text-amber-600 dark:text-amber-400 border border-amber-500/30 shadow-2xs">
        <span className="size-1.5 rounded-full bg-amber-500" aria-hidden="true" />
        INVESTIGATING
      </span>
    );
  }
  if (norm === "RESOLVED") {
    return (
      <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-bold tracking-wider uppercase bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30 shadow-2xs">
        <span className="size-1.5 rounded-full bg-emerald-500" aria-hidden="true" />
        RESOLVED
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-bold tracking-wider uppercase bg-slate-500/15 text-slate-600 dark:text-slate-400 border border-slate-500/30">
      {status}
    </span>
  );
}

type Props = { currentStatus: string; openCount: number; investigatingCount: number; resolvedCount: number; totalOccurrences: number; setStatusFilter: (status: string) => void };

export function IncidentMetrics({ currentStatus, openCount, investigatingCount, resolvedCount, totalOccurrences, setStatusFilter }: Props) {
  return (
<div className="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-5">
        <button
          type="button"
          onClick={() => setStatusFilter(currentStatus === "OPEN" ? "" : "OPEN")}
          className={`p-4 rounded-[20px] backdrop-blur-2xl border transition-all text-left cursor-pointer group active:scale-[0.99] ${
            currentStatus === "OPEN"
              ? "bg-rose-500/15 border-rose-500/40 shadow-sm"
              : "bg-white/75 dark:bg-[#08182f]/60 border-white/80 dark:border-white/10 hover:border-rose-500/40 shadow-[0_4px_20px_rgba(0,0,0,0.03)] dark:shadow-[0_4px_20px_rgba(0,0,0,0.2)]"
          }`}
          title="Filter by Open incidents"
        >
          <div className="flex items-center justify-between mb-2">
            <span className="text-[11px] font-bold uppercase tracking-wider text-slate-500 dark:text-slate-400">
              Open Incidents
            </span>
            <span className="size-2 rounded-full bg-rose-500 animate-pulse" aria-hidden="true" />
          </div>
          <div className="text-2xl md:text-3xl font-black tabular-nums text-rose-600 dark:text-rose-400">
            {openCount}
          </div>
          <span className="text-[11px] font-medium text-slate-500 dark:text-slate-400 mt-1 block">
            {currentStatus === "OPEN" ? "Filtering: active failures" : "Active application failures"}
          </span>
        </button>

        <button
          type="button"
          onClick={() => setStatusFilter(currentStatus === "INVESTIGATING" ? "" : "INVESTIGATING")}
          className={`p-4 rounded-[20px] backdrop-blur-2xl border transition-all text-left cursor-pointer group active:scale-[0.99] ${
            currentStatus === "INVESTIGATING"
              ? "bg-amber-500/15 border-amber-500/40 shadow-sm"
              : "bg-white/75 dark:bg-[#08182f]/60 border-white/80 dark:border-white/10 hover:border-amber-500/40 shadow-[0_4px_20px_rgba(0,0,0,0.03)] dark:shadow-[0_4px_20px_rgba(0,0,0,0.2)]"
          }`}
          title="Filter by Investigating incidents"
        >
          <div className="flex items-center justify-between mb-2">
            <span className="text-[11px] font-bold uppercase tracking-wider text-slate-500 dark:text-slate-400">
              Investigating
            </span>
            <AlertCircle size={14} className="text-amber-500" aria-hidden="true" />
          </div>
          <div className="text-2xl md:text-3xl font-black tabular-nums text-amber-600 dark:text-amber-400">
            {investigatingCount}
          </div>
          <span className="text-[11px] font-medium text-slate-500 dark:text-slate-400 mt-1 block">
            {currentStatus === "INVESTIGATING" ? "Filtering: under triage" : "Under active triage"}
          </span>
        </button>

        <button
          type="button"
          onClick={() => setStatusFilter(currentStatus === "RESOLVED" ? "" : "RESOLVED")}
          className={`p-4 rounded-[20px] backdrop-blur-2xl border transition-all text-left cursor-pointer group active:scale-[0.99] ${
            currentStatus === "RESOLVED"
              ? "bg-emerald-500/15 border-emerald-500/40 shadow-sm"
              : "bg-white/75 dark:bg-[#08182f]/60 border-white/80 dark:border-white/10 hover:border-emerald-500/40 shadow-[0_4px_20px_rgba(0,0,0,0.03)] dark:shadow-[0_4px_20px_rgba(0,0,0,0.2)]"
          }`}
          title="Filter by Resolved incidents"
        >
          <div className="flex items-center justify-between mb-2">
            <span className="text-[11px] font-bold uppercase tracking-wider text-slate-500 dark:text-slate-400">
              Resolved
            </span>
            <CheckCircle2 size={14} className="text-emerald-500" aria-hidden="true" />
          </div>
          <div className="text-2xl md:text-3xl font-black tabular-nums text-emerald-600 dark:text-emerald-400">
            {resolvedCount}
          </div>
          <span className="text-[11px] font-medium text-slate-500 dark:text-slate-400 mt-1 block">
            {currentStatus === "RESOLVED" ? "Filtering: mitigated" : "Mitigated incidents"}
          </span>
        </button>

        <div className="p-4 rounded-[20px] backdrop-blur-2xl bg-white/75 dark:bg-[#08182f]/60 border border-white/80 dark:border-white/10 shadow-[0_4px_20px_rgba(0,0,0,0.03)] dark:shadow-[0_4px_20px_rgba(0,0,0,0.2)]">
          <div className="flex items-center justify-between mb-2">
            <span className="text-[11px] font-bold uppercase tracking-wider text-slate-500 dark:text-slate-400">
              Total Occurrences
            </span>
            <Flame size={14} className="text-sky-500" aria-hidden="true" />
          </div>
          <div className="text-2xl md:text-3xl font-black tabular-nums text-slate-900 dark:text-white">
            {totalOccurrences.toLocaleString()}
          </div>
          <span className="text-[11px] font-medium text-slate-500 dark:text-slate-400 mt-1 block">
            Impact across all services
          </span>
        </div>
      </div>
  );
}
