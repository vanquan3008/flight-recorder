import { ArrowRight, FileClock, Layers3, Radio } from "lucide-react";
import { useState } from "react";
import { useAuth } from "../../app/auth";
import { usePagedQuery } from "../../shared/api";
import type { IncidentEvent, IncidentOccurrence, StatusChange } from "../../shared/types";
import { Badge, CopyButton, DetailLink, Empty, ErrorState, Loading, PaginationBar, TimeCell } from "../../shared/ui";
import { secondaryButtonClass } from "../../shared/ui/styles";

type Kind = "occurrences" | "telemetry" | "history";
type RecordItem = IncidentOccurrence | IncidentEvent | StatusChange;
const views = {
  occurrences: { title: "Occurrence timeline", endpoint: "occurrences", key: "incident-occurrences", Icon: Layers3 },
  telemetry: { title: "Linked telemetry", endpoint: "events", key: "incident-events", Icon: Radio },
  history: { title: "Status history", endpoint: "status-changes", key: "incident-changes", Icon: FileClock },
};

export function IncidentRecords({ incidentID, kind, totalOccurrences }: { incidentID: string; kind: Kind; totalOccurrences: number }) {
  const { credential } = useAuth();
  const view = views[kind];
  const query = usePagedQuery<RecordItem>([view.key, incidentID], `/api/v1/incidents/${incidentID}/${view.endpoint}?limit=25`, credential);
  const [page, setPage] = useState(0);
  const pages = query.data?.pages ?? [];
  const index = Math.min(page, Math.max(0, pages.length - 1));
  const rows = pages[index]?.data ?? [];
  const loaded = pages.reduce((sum, item) => sum + item.data.length, 0);
  const next = async () => {
    if (index < pages.length - 1) { setPage(index + 1); return; }
    if (query.hasNextPage && !query.isFetchingNextPage) {
      const result = await query.fetchNextPage();
      if (result.data && result.data.pages.length > pages.length) setPage(index + 1);
    }
  };
  return <section className="overflow-hidden rounded-2xl border border-line bg-surface-solid shadow-sm" aria-label={view.title}>
    <header className="flex flex-wrap items-center justify-between gap-2 border-b border-line-soft px-5 py-4">
      <h2 className="m-0 inline-flex items-center gap-2 text-sm font-semibold"><view.Icon size={15} className="text-muted" />{view.title}</h2>
      {!query.isPending && <span className="text-[11px] text-muted">{loaded} loaded{kind === "occurrences" ? ` · ${totalOccurrences.toLocaleString("en")} total occurrences` : ""}</span>}
    </header>
    {query.isPending ? <Loading /> : query.isError && !query.data ? <ErrorState error={query.error} retry={() => void query.refetch()} /> : <>
      {query.isError && <div role="alert" className="flex flex-wrap items-center justify-between gap-2 border-b border-rose-500/20 bg-danger-bg px-5 py-3 text-xs text-danger-fg"><span>Could not load the next page. Your loaded records are still available.</span><button type="button" className={secondaryButtonClass} onClick={() => void next()}>Retry next page</button></div>}
      {!rows.length ? <Empty>{kind === "occurrences" ? "No occurrences are retained for this incident." : kind === "history" ? "No status changes have been recorded." : "No telemetry events are linked to this incident."}</Empty> : <ol className="m-0 list-none divide-y divide-line-soft p-0">
        {rows.map((row, rowIndex) => kind === "history"
          ? <HistoryRow key={"id" in row ? row.id : rowIndex} change={row as StatusChange} />
          : <SignalRow key={"id" in row ? (row as IncidentOccurrence).id : `${(row as IncidentEvent).occurred_at}-${rowIndex}`} event={row as IncidentEvent} />)}
      </ol>}
      {query.limitReached && <p role="note" className="m-0 border-t border-line-soft px-5 py-3 text-xs text-muted">Loaded page limit reached. Older records are outside this view.</p>}
      <PaginationBar pagination={{ pageIndex: index, pageSize: 25, pageCount: pages.length, totalCurrentPage: rows.length,
        canPrev: index > 0, canNext: index < pages.length - 1 || Boolean(query.hasNextPage), isFetchingNext: query.isFetchingNextPage,
        onPrev: () => setPage(Math.max(0, index - 1)), onNext: () => void next(), onSelectPage: setPage }} />
    </>}
  </section>;
}

function SignalRow({ event }: { event: IncidentEvent }) {
  const message = event.error_message || event.error_type || "No error message recorded";
  return <li className="incident-record grid min-w-0 gap-3 px-5 py-3.5 hover:bg-hover md:grid-cols-[180px_minmax(0,1fr)_100px]">
    <time dateTime={event.occurred_at} className="text-[11px] text-muted"><TimeCell value={event.occurred_at} /></time>
    <div className="min-w-0">
      <div className="mb-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs"><strong className="font-semibold">{event.service_name}</strong><span className="break-words text-muted">{event.operation || "No operation recorded"}</span>{event.event_type && <span className="rounded bg-neutral-bg px-1.5 py-0.5 text-[9px] text-muted">{event.event_type}</span>}</div>
      {message.length > 180 ? <details className="group text-xs leading-relaxed"><summary className="cursor-pointer text-muted">{message.slice(0, 180)}… <span className="text-accent">Show full error</span></summary><pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-surface-sunken p-3 font-mono text-[11px]">{message}</pre><CopyButton value={message} label="Copy full error" /></details>
        : <p className="m-0 break-words text-xs leading-relaxed text-muted">{message}</p>}
    </div>
    <div className="text-xs md:text-right">{event.trace_id ? <DetailLink to={`/traces/${event.trace_id}`}>Open trace ↗</DetailLink> : <span className="text-faint">No linked trace</span>}</div>
  </li>;
}

function HistoryRow({ change }: { change: StatusChange }) {
  return <li className="incident-record flex flex-wrap items-center justify-between gap-3 px-5 py-4 hover:bg-hover">
    <div className="flex flex-wrap items-center gap-3"><time dateTime={change.changed_at}><TimeCell value={change.changed_at} /></time><span className="inline-flex items-center gap-2"><Badge value={change.previous_status} /><ArrowRight size={13} className="text-faint" /><Badge value={change.status} /></span></div>
    <span className="text-xs text-muted" title={change.actor_user_id}>{change.actor_user_id ? `user ${change.actor_user_id.slice(0, 8)}` : "system"}</span>
  </li>;
}
