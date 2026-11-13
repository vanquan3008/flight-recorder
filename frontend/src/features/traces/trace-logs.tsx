import { ChevronRight, FileText, Search } from "lucide-react";
import { useMemo, useState } from "react";
import type { LogRecord } from "../../shared/types";
import type { PagedQuery } from "../../shared/ui";
import { CopyButton, DetailLink, Empty, ErrorState, LoadMore, Loading, formatTimeParts, logBodyText } from "../../shared/ui";

export function TraceLogs({ traceID, logs, query, embedded = false }: { traceID: string; logs: LogRecord[]; query: PagedQuery; embedded?: boolean }) {
  const [search, setSearch] = useState("");
  const filtered = useMemo(() => {
    const text = search.trim().toLowerCase();
    return text ? logs.filter((log) => `${log.service_name} ${log.severity_text} ${logBodyText(log.body)}`.toLowerCase().includes(text)) : logs;
  }, [logs, search]);
  return <section aria-labelledby="trace-logs-title" className={`min-w-0 overflow-hidden bg-surface-solid ${embedded ? "border-t border-line" : "rounded-2xl border border-line shadow-sm"}`}>
    <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line-soft px-5 py-4">
      <div className="flex items-center gap-2"><FileText size={16} className="text-muted" /><h2 id="trace-logs-title" className="m-0 text-sm font-semibold">Correlated logs</h2><span className="rounded-md bg-neutral-bg px-1.5 py-0.5 text-[10px] font-medium text-muted">{logs.length} loaded</span></div>
      <div className="flex flex-wrap items-center gap-3">
        <label className="relative"><Search size={13} className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-faint" /><input aria-label="Filter loaded logs" placeholder="Filter loaded logs…" value={search} onChange={(event) => setSearch(event.target.value)} className="border border-glass-field-edge bg-glass-field text-text focus:border-accent focus:ring-2 focus:ring-accent/15 h-8 w-48 rounded-lg py-1 pr-2 pl-8 text-xs" /></label>
        <DetailLink to={`/logs?traceId=${traceID}`} className="text-xs font-medium text-accent no-underline hover:underline">Open Log explorer ↗</DetailLink>
      </div>
    </div>
    {query.isPending ? <Loading /> : query.isError ? <ErrorState error={query.error} retry={() => void query.refetch()} /> : <>
      {filtered.length ? <div className="divide-y divide-line-soft">{filtered.map((log) => <LogLine key={log.event_id} log={log} />)}</div>
        : <Empty>{search ? "No loaded logs match this filter." : "No logs are linked to this trace."}</Empty>}
      {search && <p className="m-0 border-t border-line-soft px-5 py-2 text-[11px] text-muted">{filtered.length} of {logs.length} loaded logs match. Use Log explorer to search all logs.</p>}
      {query.limitReached && <p className="m-0 px-5 py-3 text-xs text-muted">Loaded page limit reached. Open Log explorer to narrow the search.</p>}
      <LoadMore hasMore={query.hasNextPage} loading={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()} />
    </>}
  </section>;
}

function LogLine({ log }: { log: LogRecord }) {
  const timestamp = formatTimeParts(log.occurred_at);
  const severity = log.severity_text || "UNSET";
  const error = /^(ERROR|FATAL|CRITICAL)$/i.test(severity);
  const warning = /^(WARN|WARNING)$/i.test(severity);
  const body = logBodyText(log.body);
  return <details className={`group border-l-2 ${error ? "border-l-rose-500" : warning ? "border-l-amber-500" : "border-l-transparent"}`}>
    <summary className="grid cursor-pointer list-none grid-cols-[16px_minmax(0,1fr)] items-start gap-x-2 gap-y-1 px-3 py-3 hover:bg-hover sm:grid-cols-[16px_100px_160px_minmax(0,1fr)] sm:px-4 [&::-webkit-details-marker]:hidden">
      <ChevronRight size={13} className="mt-0.5 text-faint transition-transform group-open:rotate-90" />
      <time dateTime={log.occurred_at} title={timestamp?.full} className="font-mono text-[11px] leading-5 text-muted">{timestamp?.time ?? "—"}</time>
      <span className="col-start-2 flex min-w-0 items-center gap-2 sm:col-start-auto"><span className={`shrink-0 rounded px-1.5 py-0.5 text-[9px] font-semibold ${error ? "bg-rose-500/10 text-rose-700 dark:text-rose-300" : warning ? "bg-amber-500/10 text-amber-700 dark:text-amber-300" : "bg-neutral-bg text-muted"}`}>{severity}</span><span className="truncate text-[11px] font-medium" title={log.service_name}>{log.service_name}</span></span>
      <span className="col-start-2 min-w-0 truncate font-mono text-xs leading-5 text-text sm:col-start-auto" title={body}> {body.length > 300 ? `${body.slice(0, 300)}…` : body}</span>
    </summary>
    <div className="mx-4 mb-4 rounded-lg border border-line-soft bg-surface-sunken p-3">
      <div className="mb-2 flex items-center justify-between gap-3 text-[10px] text-muted"><span>{timestamp?.full}</span><CopyButton value={body} label="Copy log body" /></div>
      <pre className="m-0 max-h-72 overflow-auto whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-text">{body}</pre>
      {log.span_id && <DetailLink to={`/logs?traceId=${log.trace_id || ""}&spanId=${log.span_id}`} className="mt-3 inline-block text-[11px] text-accent">Logs for this span ↗</DetailLink>}
    </div>
  </details>;
}
