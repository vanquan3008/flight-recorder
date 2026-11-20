import { secondaryButtonClass, toolbarClass } from "../../shared/ui/styles";
import { RotateCcw, Search } from "lucide-react";
import type { FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { compactSearchParams } from "../../shared/api";
import {
  Badge,
  DetailLink,
  ErrorState,
  PageHeader,
  PagedTable,
  Panel,
  TimeWindowFilter,
  formatDuration,
  formatTime,
  useTimeWindow,
} from "../../shared/ui";
import { useTracesQuery } from "./use-traces-query";

export function TracesPage() {
  const [params, setParams] = useSearchParams();
  const window = useTimeWindow();

  const service = params.get("service") || "";
  const status = params.get("status") || "";
  const traceId = params.get("traceId") || "";
  const rawLimit = Number(params.get("limit"));

  const { query, traces, safePageIndex, pagination } = useTracesQuery(
    {
      service: service || undefined,
      status: status || undefined,
      traceId: traceId || undefined,
      from: window.from,
      to: window.to,
    },
    rawLimit || 15,
    (newSize) => {
      const next = new URLSearchParams(params);
      next.set("limit", String(newSize));
      setParams(next);
    },
  );

  const filterKey = `${service}|${status}|${traceId}|${window.from}|${window.to}|${pagination.pageSize}`;
  const hasActiveFilters = Boolean(service || status || traceId);

  const clearFilters = () => {
    const next = new URLSearchParams();
    for (const key of ["range", "from", "to", "project", "environment", "limit"]) {
      if (params.has(key)) next.set(key, params.get(key)!);
    }
    setParams(next);
  };

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setParams(compactSearchParams(event.currentTarget, params, ["range", "from", "to", "limit"]));
  };

  if (query.isError) {
    return <ErrorState error={query.error} retry={() => void query.refetch()} fullPage />;
  }

  return (
    <div className="trace-explorer-page flex h-full max-h-full min-h-0 w-full max-w-full flex-1 flex-col overflow-hidden">
      <div className="shrink-0">
        <PageHeader title="Trace explorer" action={<TimeWindowFilter {...window} />}>
          Find slow or failed requests, then inspect their spans and correlated logs.
        </PageHeader>
        <form className={`toolbar ${toolbarClass}`} key={filterKey} onSubmit={submit}>
          <input
            name="service"
            placeholder="Service"
            aria-label="Filter by service"
            defaultValue={service}
          />
          <select name="status" aria-label="Filter by status" defaultValue={status}>
            <option value="">All statuses</option>
            <option value="ERROR">ERROR</option>
            <option value="OK">OK</option>
          </select>
          <input
            name="traceId"
            placeholder="Trace ID"
            aria-label="Filter by trace ID"
            defaultValue={traceId}
          />
          <button>
            <Search size={16} /> Search traces
          </button>
          {hasActiveFilters && (
            <button
              type="button"
              onClick={clearFilters}
              className={`button-secondary ${secondaryButtonClass} h-9`}
              title="Clear search filters"
            >
              <RotateCcw size={14} aria-hidden="true" /> Clear
            </button>
          )}
        </form>
      </div>
      <Panel
        title="Recent traces"
        fit
        meta={query.isPending ? undefined : `Page ${safePageIndex + 1} · ${traces.length} traces`}
      >
        <PagedTable
          query={query}
          rowKeys={traces.map((trace) => trace.trace_id)}
          pagination={pagination}
          empty={
            hasActiveFilters ? (
              <div className="flex flex-col items-center justify-center gap-2 py-2">
                <span>No traces match those filters.</span>
                <button
                  type="button"
                  onClick={clearFilters}
                  className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-semibold ${secondaryButtonClass}`}
                >
                  <RotateCcw size={12} aria-hidden="true" />
                  Clear filters
                </button>
              </div>
            ) : (
              <>No traces match those filters. <Link to="/overview">Review the setup steps.</Link></>
            )
          }
          headers={["Started", "Service", "Duration", "Spans", "Errors", "Trace ID", ""]}
          rows={traces.map((trace) => [
            formatTime(trace.started_at),
            trace.root_service,
            formatDuration(trace.duration_nanos),
            trace.span_count,
            <Badge key={`stat-${trace.trace_id}`} value={trace.error_count ? "ERROR" : "OK"} />,
            <code key={`id-${trace.trace_id}`}>{trace.trace_id}</code>,
            <DetailLink key={`inspect-${trace.trace_id}`} to={`/traces/${trace.trace_id}`}>Inspect</DetailLink>,
          ])}
        />
      </Panel>
    </div>
  );
}

export { TraceDetailPage } from "./trace-detail";
