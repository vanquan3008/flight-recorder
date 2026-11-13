import { AlertTriangle, ArrowUpRight, Braces, Clock3, Layers3, SlidersHorizontal, X } from "lucide-react";
import type { KeyboardEvent } from "react";
import { useId, useState } from "react";
import type { Span } from "../../shared/types";
import { CopyButton, DetailLink, json } from "../../shared/ui";
import { KeyValueTable, findException } from "./span-kv-table";
import { getSpanDisplayInfo } from "./trace-format";
import { formatMs, isError } from "./trace-model";

type Props = { span: Span; traceID: string; startMs: number; durationMs: number; onClose?: () => void };
type Tab = "attributes" | "resource" | "events" | "raw";

export function SpanDetail({ span, traceID, startMs, durationMs, onClose }: Props) {
  const info = getSpanDisplayInfo(span);
  const attrs = (span.attributes as Record<string, unknown>) || {};
  const resource = (span.resource_attributes as Record<string, unknown>) || {};
  const events = Array.isArray(span.events) ? span.events as Record<string, unknown>[] : [];
  const exception = findException(span);
  const [activeTab, setActiveTab] = useState<Tab>("attributes");
  const tabID = useId();
  const tabs: { id: Tab; label: string; count?: number; Icon: typeof SlidersHorizontal }[] = [
    { id: "attributes", label: "Attributes", count: Object.keys(attrs).length, Icon: SlidersHorizontal },
    { id: "resource", label: "Resource", count: Object.keys(resource).length, Icon: Layers3 },
    ...(events.length ? [{ id: "events" as const, label: "Events", count: events.length, Icon: Clock3 }] : []),
    { id: "raw", label: "Raw", Icon: Braces },
  ];
  const changeTab = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    let next = index;
    if (event.key === "ArrowRight") next = (index + 1) % tabs.length;
    else if (event.key === "ArrowLeft") next = (index + tabs.length - 1) % tabs.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = tabs.length - 1;
    else return;
    event.preventDefault();
    setActiveTab(tabs[next].id);
    event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[role="tab"]')[next]?.focus();
  };
  return <section data-span-detail aria-label={`Span ${span.name}`}>
    <div className="flex items-center justify-between gap-3 border-b border-line-soft px-4 py-3">
      <span className="text-[11px] font-semibold uppercase tracking-wider text-muted">Span details</span>
      {onClose && <button type="button" className="rounded-md p-1 text-muted hover:bg-hover" aria-label="Close span details" onClick={onClose}><X size={15} /></button>}
    </div>
    <div className="space-y-3 px-4 py-4">
      <div className="flex flex-wrap items-center gap-2 text-[10px] font-medium">
        <span className="rounded bg-neutral-bg px-2 py-1">{span.service_name}</span>
        <span className={`rounded px-2 py-1 ${isError(span) ? "bg-rose-500/10 text-rose-600 dark:text-rose-300" : "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"}`}>{info.httpStatus ? `HTTP ${info.httpStatus} · ` : ""}{span.status_code || "UNSET"}</span>
      </div>
      <h3 className="m-0 break-words text-sm font-semibold leading-relaxed">{info.displayName}</h3>
      <p className="m-0 text-[11px] text-muted">starts at +{formatMs(startMs)} · {formatMs(durationMs)}</p>
      <div className="flex items-center justify-between gap-2 rounded-lg border border-line-soft bg-surface-sunken px-2.5 py-1.5"><span className="text-[10px] text-muted">Span ID</span><code className="min-w-0 truncate text-[10px]">{span.span_id}</code><CopyButton value={span.span_id} label="Copy span ID" /></div>
      <DetailLink to={`/logs?traceId=${traceID}&spanId=${span.span_id}`} className="inline-flex items-center gap-1.5 text-[11px] font-medium text-accent no-underline hover:underline">Logs for this span<ArrowUpRight size={12} /></DetailLink>
    </div>
    {exception && <div className="mx-4 mb-4 rounded-lg border border-rose-500/20 bg-rose-500/5 p-3">
      <div className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold text-rose-600 dark:text-rose-300"><AlertTriangle size={13} /><span className="break-all">{exception.type}</span></div>
      <p className="m-0 break-words text-xs leading-relaxed">{exception.message}</p>
      {exception.stacktrace && <details className="mt-3"><summary className="cursor-pointer text-[11px] font-medium text-muted">View stack trace</summary><pre className="m-0 mt-2 max-h-56 overflow-auto whitespace-pre-wrap break-words font-mono text-[11px] leading-relaxed">{exception.stacktrace}</pre><CopyButton value={exception.stacktrace} label="Copy stack trace" /></details>}
    </div>}
    <div role="tablist" aria-label="Span properties" className="flex overflow-x-auto border-y border-line-soft px-3">
      {tabs.map(({ id, label, count, Icon }, index) => <button key={id} type="button" role="tab" id={`${tabID}-${id}-tab`} aria-controls={`${tabID}-panel`} aria-selected={activeTab === id} tabIndex={activeTab === id ? 0 : -1}
        onClick={() => setActiveTab(id)} onKeyDown={(event) => changeTab(event, index)}
        className={`inline-flex shrink-0 items-center gap-1.5 border-0 border-b-2 border-solid px-2.5 py-3 text-[11px] font-medium ${activeTab === id ? "border-accent text-accent" : "border-transparent text-muted hover:text-text"}`}>
        <Icon size={12} />{label}{count !== undefined && <span className="text-[9px] opacity-65">{count}</span>}
      </button>)}
    </div>
    <div role="tabpanel" id={`${tabID}-panel`} aria-labelledby={`${tabID}-${activeTab}-tab`} tabIndex={0} className="min-w-0">
      {activeTab === "attributes" && <KeyValueTable key="attributes" data={attrs} placeholder="Filter span attributes…" />}
      {activeTab === "resource" && <KeyValueTable key="resource" data={resource} placeholder="Filter resource attributes…" />}
      {activeTab === "events" && <div className="divide-y divide-line-soft">{events.map((event, index) => <article key={index} className="p-3"><strong className="text-xs font-medium">{String(event.name || event.Name || `Event ${index + 1}`)}</strong><p className="m-0 mt-1 text-[10px] text-muted">{String(event.timestamp || event.Timestamp || "")}</p><KeyValueTable data={(event.attributes || event.Attributes || {}) as Record<string, unknown>} /></article>)}</div>}
      {activeTab === "raw" && <div className="p-4"><div className="mb-2 flex items-center justify-between text-[11px] text-muted"><span>Full span payload</span><CopyButton value={json(span)} label="Copy full payload" /></div><pre className="m-0 overflow-auto whitespace-pre-wrap break-words font-mono text-[11px] leading-relaxed">{json(span)}</pre></div>}
    </div>
  </section>;
}
