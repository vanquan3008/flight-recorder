import { secondaryButtonClass } from "../../shared/ui/styles";
import { Columns, Radio, Rows } from "lucide-react";
import type { FormEvent } from "react";
import { useCallback, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useAuth } from "../../shared/session";
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
import { LogFilters } from "./log-filters";
import { LogTable } from "./log-table";

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

      <div className="log-toolbar-bar mb-4 flex w-full flex-wrap items-start justify-between gap-3 max-md:mb-3">
        <LogFilters params={params} formKey={filterNames.map((name) => params.get(name) || "").join("|")} active={active}
          onSubmit={submit} onPick={pick} onClear={clear} />

        <div className="log-toolbar-actions inline-flex items-center gap-2 max-md:hidden">
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
