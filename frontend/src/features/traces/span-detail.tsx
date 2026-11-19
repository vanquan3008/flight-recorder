import { AlertTriangle, ArrowUpRight, Braces, Clock3, Layers3, Link2, SlidersHorizontal, X } from "lucide-react";
import type { KeyboardEvent } from "react";
import { Fragment, useId, useState } from "react";
import type { Span } from "../../shared/types";
import { CopyButton, DetailLink, json } from "../../shared/ui";
import { KeyValueTable, findException } from "./span-kv-table";
import { SpanLinks, spanLinks } from "./span-links";
import { getSpanDisplayInfo, spanKindLabel } from "./trace-format";
import { formatMs, isError } from "./trace-model";

type Props = { span: Span; traceID: string; startMs: number; durationMs: number; onClose?: () => void };
type Tab = "attributes" | "resource" | "events" | "links" | "raw";

export function SpanDetail({ span, traceID, startMs, durationMs, onClose }: Props) {
  const info = getSpanDisplayInfo(span);
  const attrs = (span.attributes as Record<string, unknown>) || {};
  const resource = (span.resource_attributes as Record<string, unknown>) || {};
  const events = Array.isArray(span.events) ? span.events as Record<string, unknown>[] : [];
  const links = spanLinks(span.links);
  const exception = findException(span);
  const facts = ([
    ["Kind", spanKindLabel(span.kind)], ["Service version", span.service_version],
    ["Scope", [span.scope_name, span.scope_version].filter(Boolean).join(" ")], ["Status message", span.status_message],
  ] as [string, string | undefined][]).filter((fact): fact is [string, string] => Boolean(fact[1]));
  const dropped = ([["attributes", span.dropped_attributes_count], ["events", span.dropped_events_count], ["links", span.dropped_links_count]] as [string, number | undefined][])
    .filter(([, count]) => (count ?? 0) > 0);
  const [activeTab, setActiveTab] = useState<Tab>("attributes");
  const tabID = useId();
  const tabs: { id: Tab; label: string; count?: number; Icon: typeof SlidersHorizontal }[] = [
    { id: "attributes", label: "Attributes", count: Object.keys(attrs).length, Icon: SlidersHorizontal },
    { id: "resource", label: "Resource", count: Object.keys(resource).length, Icon: Layers3 },
    ...(events.length ? [{ id: "events" as const, label: "Events", count: events.length, Icon: Clock3 }] : []),
    ...(links.length ? [{ id: "links" as const, label: "Links", count: links.length, Icon: Link2 }] : []),
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
    {onClose && <div className="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-line-soft bg-surface-solid px-4 py-1.5">
      <h3 className="m-0 text-xs font-semibold text-muted">Span details</h3>
      <button type="button" className="grid size-8 shrink-0 place-items-center rounded-md text-muted hover:bg-hover hover:text-text" aria-label="Close span details" onClick={onClose}><X size={16} /></button>
    </div>}
    <div className="space-y-2.5 px-4 py-3.5">
      <div className="flex flex-wrap items-center gap-2 text-[11px] font-medium">
        <span className="min-w-0 rounded bg-neutral-bg px-2 py-1 wrap-anywhere">{span.service_name}</span>
        <span className={`rounded px-2 py-1 ${isError(span) ? "bg-rose-500/10 text-rose-600 dark:text-rose-300" : "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"}`}>{info.httpStatus ? `HTTP ${info.httpStatus} · ` : ""}{span.status_code || "UNSET"}</span>
      </div>
      <h3 className="m-0 text-sm font-semibold leading-snug wrap-anywhere">{info.displayName}</h3>
      <p className="m-0 text-xs text-muted">starts at +{formatMs(startMs)} · {formatMs(durationMs)}</p>
      {facts.length > 0 && <dl aria-label="Span facts" className="m-0 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-[11px]">{facts.map(([label, value]) => <Fragment key={label}><dt className="text-muted">{label}</dt><dd className="m-0 wrap-anywhere">{value}</dd></Fragment>)}</dl>}
      {dropped.length > 0 && <p role="note" className="m-0 rounded-lg border border-amber-500/30 bg-amber-500/5 px-2.5 py-1.5 text-[11px] text-muted">The SDK dropped {dropped.map(([name, count]) => `${count} ${name}`).join(", ")} before export.</p>}
      <div className="flex items-center gap-2 rounded-lg border border-line-soft bg-surface-sunken px-2.5 py-1"><span className="shrink-0 text-[11px] text-muted">Span ID</span><code className="min-w-0 flex-1 truncate text-[11px]">{span.span_id}</code><CopyButton value={span.span_id} label="Copy span ID" compact /></div>
      <DetailLink to={`/logs?traceId=${traceID}&spanId=${span.span_id}`} className="inline-flex items-center gap-1.5 text-[11px] font-medium text-accent no-underline hover:underline">Logs for this span<ArrowUpRight size={12} /></DetailLink>
    </div>
    {exception && <div className="mx-4 mb-4 rounded-lg border border-rose-500/20 bg-rose-500/5 p-3">
      <div className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold text-rose-600 dark:text-rose-300"><AlertTriangle size={13} className="shrink-0" /><span className="wrap-anywhere">{exception.type}</span></div>
      <p className="m-0 text-xs leading-relaxed wrap-anywhere">{exception.message}</p>
      {exception.stacktrace && <details className="mt-3"><summary className="cursor-pointer text-xs font-medium text-muted">View stack trace</summary><pre className="m-0 mt-2 max-h-56 overflow-y-auto whitespace-pre-wrap font-mono text-xs leading-relaxed wrap-anywhere">{exception.stacktrace}</pre><CopyButton value={exception.stacktrace} label="Copy stack trace" compact /></details>}
    </div>}
    <div role="tablist" aria-label="Span properties" className="flex flex-wrap border-y border-line-soft px-3">
      {tabs.map(({ id, label, count, Icon }, index) => <button key={id} type="button" role="tab" id={`${tabID}-${id}-tab`} aria-controls={`${tabID}-panel`} aria-selected={activeTab === id} tabIndex={activeTab === id ? 0 : -1}
        onClick={() => setActiveTab(id)} onKeyDown={(event) => changeTab(event, index)}
        className={`inline-flex min-h-9 items-center gap-1 border-0 border-b-2 border-solid px-2 text-xs font-medium ${activeTab === id ? "border-accent text-accent" : "border-transparent text-muted hover:text-text"}`}>
        <Icon size={12} aria-hidden="true" className="max-[359px]:hidden" />{label}{count !== undefined && <span className="text-[10px] tabular-nums text-muted">{count}</span>}
      </button>)}
    </div>
    <div role="tabpanel" id={`${tabID}-panel`} aria-labelledby={`${tabID}-${activeTab}-tab`} tabIndex={0} className="min-w-0">
      {activeTab === "attributes" && <KeyValueTable key="attributes" data={attrs} placeholder="Filter span attributes…" />}
      {activeTab === "resource" && <KeyValueTable key="resource" data={resource} placeholder="Filter resource attributes…" />}
      {activeTab === "events" && <div className="divide-y divide-line-soft">{events.map((event, index) => <article key={index} className="px-4 py-3 wrap-anywhere"><strong className="text-xs font-medium">{String(event.name || event.Name || `Event ${index + 1}`)}</strong><p className="m-0 mt-1 text-[11px] text-muted">{String(event.timestamp || event.Timestamp || "")}</p><div className="-mx-4 mt-2"><KeyValueTable data={(event.attributes || event.Attributes || {}) as Record<string, unknown>} /></div></article>)}</div>}
      {activeTab === "links" && <SpanLinks links={links} />}
      {activeTab === "raw" && <div className="p-4"><div className="mb-2 flex items-center justify-between text-xs text-muted"><span>Full span payload</span><CopyButton value={json(span)} label="Copy full payload" compact /></div><pre className="m-0 whitespace-pre-wrap font-mono text-xs leading-relaxed wrap-anywhere">{json(span)}</pre></div>}
    </div>
  </section>;
}
