import { IncidentMetrics, StatusBadge } from "./incident-metrics";
import { ArrowRight, RotateCcw, Search, Server } from "lucide-react";
import type { FormEvent } from "react";
import { useSearchParams } from "react-router-dom";
import { compactSearchParams } from "../../shared/api";
import {
  Badge,
  DetailLink,
  ErrorState,
  PageHeader,
  PagedTable,
  Panel,
  TimeCell,
  TimeWindowFilter,
  useTimeWindow,
} from "../../shared/ui";
import { useIncidentsQuery } from "./use-incidents-query";

export function OperationsPage() {
  const [params, setParams] = useSearchParams();
  const window = useTimeWindow();

  const currentService = params.get("service") || "";
  const currentStatus = params.get("status") || "";

  const { query, incidents, counts } = useIncidentsQuery({
    service: currentService,
    status: currentStatus,
    from: window.from,
    to: window.to,
  });

  const setStatusFilter = (nextStatus: string) => {
    const next = new URLSearchParams(params);
    if (nextStatus) next.set("status", nextStatus);
    else next.delete("status");
    setParams(next);
  };

  const clearAllFilters = () => {
    const next = new URLSearchParams();
    for (const key of ["range", "from", "to", "project", "environment"]) {
      if (params.has(key)) next.set(key, params.get(key)!);
    }
    setParams(next);
  };

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setParams(compactSearchParams(event.currentTarget, params, ["range", "from", "to", "project", "environment"]));
  };

  if (query.isError) return <ErrorState error={query.error} retry={() => void query.refetch()} fullPage />;

  return (
    <div className="incidents-page flex h-full max-h-full min-h-0 w-full max-w-full flex-1 flex-col overflow-hidden">
      <div className="shrink-0">
        <PageHeader title="Incidents" action={<TimeWindowFilter {...window} />}>
          Investigate application failures from their timeline and correlated telemetry.
        </PageHeader>

        <IncidentMetrics
          currentStatus={currentStatus}
          openCount={counts.open}
          investigatingCount={counts.investigating}
          resolvedCount={counts.resolved}
          totalOccurrences={counts.totalOccurrences}
          setStatusFilter={setStatusFilter}
        />

        <div className="flex flex-col md:flex-row md:items-center justify-between gap-3 mb-3.5">
          <div
            className="inline-flex items-center gap-1.5 p-1 rounded-full backdrop-blur-2xl bg-white/70 dark:bg-white/5 border border-white/80 dark:border-white/10 shadow-xs overflow-x-auto"
            role="tablist"
            aria-label="Filter incidents by status"
          >
            {[
              { key: "", label: "All", count: incidents.length },
              { key: "OPEN", label: "Open", count: counts.open },
              { key: "INVESTIGATING", label: "Investigating", count: counts.investigating },
              { key: "RESOLVED", label: "Resolved", count: counts.resolved },
              { key: "IGNORED", label: "Ignored", count: counts.ignored },
            ].map((chip) => {
              const active = currentStatus === chip.key;
              return (
                <button
                  key={chip.key}
                  type="button"
                  role="tab"
                  aria-selected={active}
                  onClick={() => setStatusFilter(chip.key)}
                  className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-semibold whitespace-nowrap transition-all cursor-pointer ${
                    active
                      ? "bg-gradient-to-r from-sky-500 to-blue-600 text-white shadow-xs"
                      : "text-slate-600 dark:text-slate-300 hover:text-slate-950 dark:hover:text-white hover:bg-white/60 dark:hover:bg-white/10"
                  }`}
                >
                  <span>{chip.label}</span>
                  {chip.count > 0 && (
                    <span
                      className={`px-1.5 py-0.2 rounded-full text-[10px] font-mono ${
                        active ? "bg-white/20 text-white" : "bg-black/5 dark:bg-white/10 text-slate-500 dark:text-slate-400"
                      }`}
                    >
                      {chip.count}
                    </span>
                  )}
                </button>
              );
            })}
          </div>

          <form className="flex items-center gap-2" onSubmit={submit}>
            <div className="relative inline-flex items-center">
              <Search size={14} className="pointer-events-none absolute left-3 text-slate-400" aria-hidden="true" />
              <input
                name="service"
                placeholder="Filter by service…"
                aria-label="Filter by service"
                defaultValue={currentService}
                key={currentService}
                className="h-[34px] w-48 md:w-56 rounded-full bg-white/75 dark:bg-white/5 border border-white/80 dark:border-white/10 shadow-[inset_0_1px_1px_rgba(255,255,255,0.7)] dark:shadow-[inset_0_1px_1px_rgba(255,255,255,0.08)] pl-8 pr-3 text-xs font-medium text-slate-900 dark:text-white placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-sky-500/35"
              />
            </div>

            <select
              name="status"
              aria-label="Filter by status"
              value={currentStatus}
              onChange={(e) => setStatusFilter(e.target.value)}
              className="h-[34px] rounded-full bg-white/75 dark:bg-white/5 border border-white/80 dark:border-white/10 shadow-[inset_0_1px_1px_rgba(255,255,255,0.7)] dark:shadow-[inset_0_1px_1px_rgba(255,255,255,0.08)] px-3 text-xs font-semibold text-slate-800 dark:text-slate-200 outline-none cursor-pointer hidden sm:inline-flex"
            >
              <option value="" className="bg-white text-slate-900 dark:bg-[#0b172b] dark:text-slate-100">
                All statuses
              </option>
              <option value="OPEN" className="bg-white text-slate-900 dark:bg-[#0b172b] dark:text-slate-100">
                OPEN
              </option>
              <option value="INVESTIGATING" className="bg-white text-slate-900 dark:bg-[#0b172b] dark:text-slate-100">
                INVESTIGATING
              </option>
              <option value="RESOLVED" className="bg-white text-slate-900 dark:bg-[#0b172b] dark:text-slate-100">
                RESOLVED
              </option>
              <option value="IGNORED" className="bg-white text-slate-900 dark:bg-[#0b172b] dark:text-slate-100">
                IGNORED
              </option>
            </select>

            <button
              type="submit"
              className="inline-flex items-center gap-1.5 h-[34px] px-4 rounded-full text-xs font-bold bg-sky-500 hover:bg-sky-600 text-white transition-all shadow-xs cursor-pointer active:scale-95"
            >
              <Search size={13} aria-hidden="true" />
              Apply filters
            </button>

            {(currentService || currentStatus) && (
              <button
                type="button"
                onClick={clearAllFilters}
                title="Clear all filters"
                className="inline-flex items-center gap-1 h-[34px] px-3 rounded-full text-xs font-semibold text-slate-500 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white transition-colors cursor-pointer"
              >
                <RotateCcw size={12} aria-hidden="true" />
                Clear
              </button>
            )}
          </form>
        </div>
      </div>

      {/* Main Incident Timeline Table */}
      <Panel
        fit
        title="Incident timeline"
        meta={query.isPending ? undefined : `${incidents.length} loaded`}
        className="flex min-h-0 flex-1 flex-col overflow-hidden"
      >
        <PagedTable
          query={query}
          empty={
            currentService || currentStatus ? (
              <div className="flex flex-col items-center justify-center gap-2 py-2">
                <span>No incidents match the active filters.</span>
                <button
                  type="button"
                  onClick={clearAllFilters}
                  className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-semibold bg-white/70 dark:bg-white/5 border border-slate-200/80 dark:border-white/10 text-slate-700 dark:text-sky-300 hover:text-sky-600 dark:hover:text-white transition-all cursor-pointer"
                >
                  <RotateCcw size={12} aria-hidden="true" />
                  Clear filters
                </button>
              </div>
            ) : (
              "No incidents yet."
            )
          }
          headers={["Last seen", "Severity", "Status", "Title", "Occurrences", ""]}
          rows={incidents.map((incident) => [
            <TimeCell key={`time-${incident.id}`} value={incident.last_seen_at} />,
            <Badge key={`sev-${incident.id}`} value={incident.severity} />,
            <StatusBadge key={`stat-${incident.id}`} status={incident.status} />,
            <div key={`title-${incident.id}`} className="flex flex-col gap-1 min-w-0 flex-1 py-0.5">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-[11px] font-mono font-bold bg-sky-500/10 text-sky-700 dark:text-sky-300 border border-sky-500/20">
                  <Server size={11} className="shrink-0" aria-hidden="true" />
                  {incident.primary_service}
                </span>
                {incident.trace_id && (
                  <span className="text-[10px] text-slate-500 dark:text-slate-400 font-mono">
                    trace:{incident.trace_id.slice(0, 7)}
                  </span>
                )}
              </div>
              <DetailLink
                to={`/incidents/${incident.id}`}
                className="text-[13px] font-semibold text-slate-900 dark:text-slate-100 hover:text-sky-600 dark:hover:text-sky-400 transition-colors line-clamp-1 no-underline"
                title={incident.title}
              >
                {incident.title}
              </DetailLink>
              {incident.error_message && incident.error_message !== incident.title && (
                <p className="text-[11px] text-slate-500 dark:text-slate-400 line-clamp-1 m-0 font-mono">
                  {incident.error_message}
                </p>
              )}
            </div>,
            <div
              key={`occ-${incident.id}`}
              className="flex items-center gap-1.5"
              title={`${incident.occurrence_count.toLocaleString()} failure occurrences`}
            >
              <span
                className={`inline-flex items-center justify-center min-w-[32px] px-2.5 py-0.5 rounded-full text-xs font-mono font-bold tabular-nums ${
                  incident.occurrence_count >= 50
                    ? "bg-rose-500/15 text-rose-600 dark:text-rose-400 border border-rose-500/30 shadow-2xs"
                    : incident.occurrence_count >= 5
                    ? "bg-amber-500/15 text-amber-600 dark:text-amber-400 border border-amber-500/30"
                    : "bg-slate-100 dark:bg-white/5 text-slate-600 dark:text-slate-400 border border-slate-200/60 dark:border-white/10"
                }`}
              >
                {incident.occurrence_count.toLocaleString()}
              </span>
              <span className="text-[11px] text-slate-500 dark:text-slate-400 hidden md:inline">events</span>
            </div>,
            <DetailLink
              key={`action-${incident.id}`}
              to={`/incidents/${incident.id}`}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-semibold bg-white/70 dark:bg-white/5 border border-slate-200/80 dark:border-white/10 text-slate-700 dark:text-sky-300 hover:text-sky-600 dark:hover:text-white hover:border-sky-500/40 shadow-2xs hover:shadow-xs transition-all no-underline group"
              aria-label="Inspect"
            >
              <span>Inspect</span>
              <ArrowRight size={13} className="shrink-0 transition-transform group-hover:translate-x-0.5" aria-hidden="true" />
            </DetailLink>,
          ])}
        />
      </Panel>
    </div>
  );
}

export { IncidentPage } from "./incident-page";
