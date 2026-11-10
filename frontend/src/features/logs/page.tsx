import { secondaryButtonClass, toolbarClass } from "../../shared/ui/styles";
import { Columns, Radio, Rows, Search, X } from "lucide-react";
import type { FormEvent } from "react";
import { useCallback, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useAuth } from "../../app/auth";
import { compactSearchParams, pageItems, queryString, useLiveStream, usePagedQuery } from "../../shared/api";
import type { LogRecord } from "../../shared/types";
import {
  Empty,
  ErrorState,
  Loading,
  PageHeader,
  Panel,
  TimeWindowFilter,
  useTimeWindow,
} from "../../shared/ui";
import { matchesLogFilters, mergeLogs } from "./log-model";
import { LogTable } from "./log-table";

const severities = ["TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"];
const filterNames = ["service", "severity", "search", "traceId", "spanId"];

export function LogsPage() {
  const { credential } = useAuth();
  const [params, setParams] = useSearchParams();
  const window = useTimeWindow();
  const [liveTail, setLiveTail] = useState(false);
  const [liveLogs, setLiveLogs] = useState<LogRecord[]>([]);
  const [density, setDensity] = useState<"compact" | "expanded">("compact");

  const onLiveLog = useCallback((newLog: LogRecord) => {
    setLiveLogs((prev) => [newLog, ...prev.filter((item) => item.event_id !== newLog.event_id)].slice(0, 100));
  }, []);

  const { connected, error: streamError } = useLiveStream({
    enabled: liveTail,
    credential,
    onLog: onLiveLog,
  });

  const traceID = params.get("traceId") || "";
  const anyTime = Boolean(traceID) && ((!params.has("range") && !params.has("from") && !params.has("to")) || params.get("range") === "any");
  const search = params.get("search") || "";
  const longWindow = !anyTime && window.duration > 24 * 60 * 60_000;
  const values = Object.fromEntries(filterNames.map((name) => [name, params.get(name) || undefined]));
  const path = `/api/v1/logs?${queryString({ ...values, from: anyTime ? undefined : window.from, to: anyTime ? undefined : window.to })}`;
  const query = usePagedQuery<LogRecord>(["logs"], path, credential);
  const pagedLogs = pageItems(query.data?.pages);
  const filteredLive = liveLogs.filter((log) => matchesLogFilters(log, params));
  const logs = liveTail ? mergeLogs(filteredLive, pagedLogs) : pagedLogs;
  const active = filterNames.filter((name) => params.get(name));

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setParams(compactSearchParams(event.currentTarget, params, ["range", "from", "to"]));
  };
  const pick = (severity: string) => {
    const next = new URLSearchParams(params);
    if (severity) next.set("severity", severity); else next.delete("severity");
    setParams(next);
  };
  const clear = () => setParams(new URLSearchParams([...params].filter(([key]) => !filterNames.includes(key))));

  return (
    <div className="logs-page flex h-full max-h-full min-h-0 w-full max-w-full flex-1 flex-col overflow-hidden">
      <PageHeader
        title="Log explorer"
        action={
          <div className="header-controls flex flex-wrap items-center justify-end gap-3 log-header-controls gap-2.5">
            <button
              type="button"
              className={`button-secondary ${secondaryButtonClass} compact px-2.5 py-1 text-[11.5px] live-tail-btn h-8 rounded-full px-3 py-0 ${liveTail ? "border-accent bg-accent/12 text-accent active" : ""}`}
              aria-pressed={liveTail}
              onClick={() => {
                setLiveTail(!liveTail);
                if (liveTail) setLiveLogs([]);
              }}
            >
              <span className={`live-dot inline-block size-1.75 rounded-full ${connected ? "bg-emerald-500 shadow-[0_0_8px_#10b981] motion-safe:animate-pulse connected" : "bg-muted"}`} />
              <Radio size={13} aria-hidden="true" />
              {liveTail ? (connected ? "Live streaming" : streamError ? "Stream unavailable" : "Connecting...") : "Live tail"}
            </button>
            <TimeWindowFilter
              range={anyTime ? "any" : window.range}
              select={window.select}
              allowAny={Boolean(traceID)}
              from={window.from}
              to={window.to}
            />
          </div>
        }
      >
        Read logs on their own or follow a log to the trace and span that produced it.
      </PageHeader>

      {liveTail && (
        <p className="muted text-muted log-note -mt-1 mb-4 text-xs" role="status">
          Live events appear above historical results. The latest 100 live events are retained; all search filters apply.
        </p>
      )}
      {streamError && <ErrorState error={streamError} />}

      <div className="log-toolbar-bar mb-4 flex w-full flex-wrap items-center justify-between gap-3 max-[960px]:flex-col max-[960px]:items-stretch">
        <div className="log-toolbar-left flex flex-1 flex-wrap items-center gap-2.5">
          <div className="severity-chips inline-flex h-9 shrink-0 items-center gap-0.5 self-start rounded-xl border border-glass-field-edge bg-glass-field p-0.75" role="group" aria-label="Quick severity filter">
            {["", ...severities.slice(2)].map((severity) => {
              const selected = (params.get("severity") || "") === severity;
              const toneColor = !selected && (
                severity === "ERROR" || severity === "FATAL"
                  ? "text-rose-500"
                  : severity === "WARN"
                    ? "text-amber-500"
                    : severity === "INFO"
                      ? "text-sky-500"
                      : "text-muted"
              );
              return (
                <button
                  type="button"
                  key={severity || "all"}
                  className={`severity-chip inline-flex h-full cursor-pointer items-center justify-center rounded-lg border px-3 text-[11px] leading-none font-bold tracking-wide uppercase transition-colors hover:bg-glass-hover hover:text-text ${
                    selected
                      ? "border-btn-border bg-btn text-btn-text"
                      : `border-transparent ${toneColor || "text-muted"}`
                  } ${severity}`}
                  aria-pressed={selected}
                  onClick={() => pick(severity)}
                >
                  {severity || "All"}
                </button>
              );
            })}
          </div>

          <form
            className={`toolbar ${toolbarClass} log-filters m-0 flex flex-1 min-w-0 max-w-full flex-wrap items-center gap-2 [&_input]:h-9 [&_input]:min-w-0 [&_input]:rounded-lg [&_input]:border [&_input]:border-glass-field-edge [&_input]:bg-glass-field [&_input]:px-3 [&_input]:text-[12.5px] [&_input:focus]:border-accent [&_input:focus]:ring-2 [&_input:focus]:ring-accent/15 [&_input:focus]:outline-none [&_input[name=service]]:max-w-[170px] [&_input[name=service]]:flex-[1_1_130px] [&_input[name=search]]:max-w-[360px] [&_input[name=search]]:flex-[2_1_200px] [&_input[name=traceId]]:max-w-[190px] [&_input[name=traceId]]:flex-[1.2_1_140px] [&_input[name=traceId]]:font-mono [&_input[name=spanId]]:max-w-[140px] [&_input[name=spanId]]:flex-[1_1_110px] [&_input[name=spanId]]:font-mono [&_button]:h-9 [&_button]:shrink-0 [&_button]:px-4 [&_button:not(.button-secondary)]:inline-flex [&_button:not(.button-secondary)]:items-center [&_button:not(.button-secondary)]:gap-1.5 [&_button:not(.button-secondary)]:rounded-lg [&_button:not(.button-secondary)]:bg-btn [&_button:not(.button-secondary)]:text-xs [&_button:not(.button-secondary)]:font-semibold [&_button:not(.button-secondary)]:text-btn-text max-[960px]:grid max-[960px]:w-full max-[960px]:grid-cols-2 max-[960px]:[&_input]:max-w-none max-[960px]:[&_input[name=search]]:col-span-full`}
            key={filterNames.map((name) => params.get(name) || "").join("|")}
            onSubmit={submit}
          >
            <input
              name="service"
              placeholder="Service"
              aria-label="Filter logs by service"
              defaultValue={params.get("service") || ""}
            />
            <input type="hidden" name="severity" value={params.get("severity") || ""} />
            <input
              name="search"
              placeholder="Search body text"
              aria-label="Search log body"
              defaultValue={search}
            />
            <input
              name="traceId"
              placeholder="Trace ID"
              aria-label="Filter logs by trace ID"
              defaultValue={traceID}
            />
            <input
              name="spanId"
              placeholder="Span ID"
              aria-label="Filter logs by span ID"
              defaultValue={params.get("spanId") || ""}
            />
            <button type="submit">
              <Search size={15} /> Search logs
            </button>
            {active.length > 0 && (
              <button type="button" className={`button-secondary ${secondaryButtonClass}`} onClick={clear}>
                <X size={15} /> Clear filters
              </button>
            )}
          </form>
        </div>

        <div className="log-toolbar-actions inline-flex items-center gap-2">
          <button
            type="button"
            className={`density-toggle-btn ${secondaryButtonClass} h-9 ${density === "expanded" ? "border-accent bg-accent/12 text-accent active" : "text-muted"}`}
            onClick={() => setDensity(density === "compact" ? "expanded" : "compact")}
            title={density === "compact" ? "Expand all log details" : "Collapse to compact single-line view"}
            aria-label={density === "compact" ? "Switch to expanded mode" : "Switch to compact mode"}
          >
            {density === "compact" ? <Rows size={14} /> : <Columns size={14} />}
            {density === "compact" ? "Expand all" : "Compact view"}
          </button>
        </div>
      </div>

      {search && (anyTime || longWindow) && (
        <p className="muted text-muted log-note -mt-1 mb-4 text-xs" role="note">
          Body search needs a time range of 24 hours or less. Choose a shorter range if the request is rejected.
        </p>
      )}

      <Panel
        fit
        className="log-panel flex min-h-0 flex-1 flex-col overflow-hidden"
        title="Logs"
        meta={
          query.isPending
            ? undefined
            : `${logs.length} loaded${liveTail && liveLogs.length > 0 ? ` (${liveLogs.length} live)` : ""}${query.hasNextPage ? " · scroll for more" : ""}`
        }
      >
        {query.isPending ? (
          <Loading />
        ) : query.isError ? (
          <ErrorState error={query.error} retry={() => void query.refetch()} />
        ) : logs.length === 0 ? (
          <Empty>No logs match these filters in the selected time range. Widen the range or clear filters.</Empty>
        ) : (
          <LogTable key={path} logs={logs} expanded={density === "expanded"}
            hasMore={query.hasNextPage} loading={query.isFetchingNextPage} limitReached={query.limitReached}
            onLoadMore={query.fetchNextPage} />
        )}
      </Panel>
    </div>
  );
}
