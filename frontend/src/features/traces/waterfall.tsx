import { ChevronDown, ChevronRight, CircleAlert, Layers3, Search, X } from "lucide-react";
import type { CSSProperties, ReactNode } from "react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useSearchParams } from "react-router-dom";
import { BottomSheet, Empty, useMediaQuery } from "../../shared/ui";
import { SpanDetail } from "./span-detail";
import { getSpanDisplayInfo } from "./trace-format";
import { type TraceModel, axisTicks, formatMs, isError, visibleRows } from "./trace-model";
import { useVirtualList } from "./use-virtual-list";

type Props = { model: TraceModel; traceID: string; focus: string; onFocus: (service: string) => void; children?: ReactNode };
const ROW_HEIGHT = 56;
const estimateRowHeight = () => ROW_HEIGHT;
const columns = "grid grid-cols-[minmax(0,1fr)_26%_3.75rem] items-center gap-2 @min-[36rem]/waterfall:grid-cols-[minmax(0,42%)_minmax(0,1fr)_4.5rem]";

export function Waterfall({ model, traceID, focus, onFocus, children }: Props) {
  const { state } = useLocation();
  const [params, setParams] = useSearchParams();
  const selected = params.get("spanId")?.toLowerCase() ?? "";
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [search, setSearch] = useState("");
  const [errorsOnly, setErrorsOnly] = useState(false);
  const viewportRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const selectedButton = useRef<HTMLButtonElement | null>(null);
  const useSheet = useMediaQuery("(width < 80rem)");
  const selectedRow = model.rows.find((row) => row.span.span_id === selected);
  const filtering = Boolean(search.trim()) || errorsOnly;
  const rows = useMemo(() => {
    const source = filtering ? model.rows : visibleRows(model.rows, collapsed);
    const text = search.trim().toLowerCase();
    return source.filter((row) => (!errorsOnly || isError(row.span)) && (!text || `${row.span.name} ${row.span.service_name} ${row.span.span_id}`.toLowerCase().includes(text)));
  }, [model.rows, collapsed, search, errorsOnly, filtering]);
  const ticks = useMemo(() => axisTicks(model.totalMs), [model.totalMs]);
  const { virtualItems, totalHeight, isVirtualized } = useVirtualList({ count: rows.length, estimateHeight: estimateRowHeight, containerRef: viewportRef, overscan: 8 });
  useEffect(() => {
    if (!selected) return;
    const viewport = viewportRef.current;
    const index = rows.findIndex((row) => row.span.span_id === selected);
    if (!viewport || index < 0) return;
    const top = index * ROW_HEIGHT;
    if (top < viewport.scrollTop || top + ROW_HEIGHT > viewport.scrollTop + viewport.clientHeight) viewport.scrollTop = top;
  }, [rows, selected]);
  const setSelected = (id: string) => {
    const next = new URLSearchParams(params);
    if (id) next.set("spanId", id); else next.delete("spanId");
    setParams(next, { replace: true, state });
  };
  const resetScroll = () => { if (viewportRef.current) viewportRef.current.scrollTop = 0; };
  const toggle = (id: string) => setCollapsed((current) => {
    const next = new Set(current);
    if (!next.delete(id)) next.add(id);
    return next;
  });
  const selectSpan = (id: string, button: HTMLButtonElement | null) => {
    selectedButton.current = button;
    button?.focus({ preventScroll: true });
    setSelected(selected === id ? "" : id);
  };
  const closeInspector = () => {
    setSelected("");
    const highlighted = viewportRef.current?.querySelector<HTMLButtonElement>(".wf-main[aria-pressed='true']");
    const opener = highlighted ?? (selectedButton.current?.isConnected ? selectedButton.current : searchRef.current);
    opener?.focus({ preventScroll: true });
  };
  const renderRow = (row: typeof rows[0], top?: number) => {
    const id = row.span.span_id;
    const failed = isError(row.span);
    const dim = Boolean(focus) && row.span.service_name !== focus;
    const info = getSpanDisplayInfo(row.span);
    const rowStyle = { "--svc": `var(--svc-${row.serviceIndex % 6})`, "--wf-depth": Math.min(row.depth, 8), height: ROW_HEIGHT,
      ...(top !== undefined ? { position: "absolute", top, left: 0, right: 0 } : {}) } as CSSProperties;
    return <li key={id} data-span-id={id} className={`wf-row border-b border-line-soft ${dim ? "dim opacity-50" : ""} ${selected === id ? "selected bg-hover shadow-[inset_3px_0_var(--color-accent)]" : "hover:bg-hover"} ${failed ? "failed" : ""}`} style={rowStyle}>
      <div className={`${columns} h-full px-3 sm:px-4`}>
        <div className="flex min-w-0 items-center gap-1 pl-[calc(var(--wf-depth)*0.25rem)] @min-[36rem]/waterfall:pl-[calc(var(--wf-depth)*0.625rem)]">
          {row.hasChildren && !filtering ? <button type="button" className="flex h-8 w-5 shrink-0 items-center justify-center rounded text-muted hover:bg-hover hover:text-text" aria-expanded={!collapsed.has(id)} aria-label={`${collapsed.has(id) ? "Expand" : "Collapse"} ${row.span.name}`} onClick={() => toggle(id)}>{collapsed.has(id) ? <ChevronRight size={14} /> : <ChevronDown size={14} />}</button> : <span className="w-5 shrink-0" />}
          <button type="button" className="wf-main flex min-w-0 flex-1 flex-col items-start gap-1 py-1 text-left" aria-pressed={selected === id} aria-haspopup={useSheet ? "dialog" : undefined} onClick={(event) => selectSpan(id, event.currentTarget)}>
            <span className="flex max-w-full items-center gap-1.5 text-[11px] leading-none"><i className="size-1.5 shrink-0 rounded-full bg-(--svc)" aria-hidden="true" /><span className="truncate text-muted" title={row.span.service_name}>{row.span.service_name}</span>{failed && <span className="inline-flex shrink-0 items-center gap-0.5 text-rose-600 dark:text-rose-300"><CircleAlert size={10} />Error</span>}</span>
            <span className="wf-name w-full truncate text-xs font-semibold leading-tight text-text @min-[36rem]/waterfall:text-[13px]" title={info.displayName}>{info.displayName}</span>
          </button>
        </div>
        <div className="wf-rail relative h-full cursor-pointer overflow-hidden" aria-hidden="true" onClick={(event) => selectSpan(id, event.currentTarget.closest("li")?.querySelector<HTMLButtonElement>(".wf-main") ?? null)}>
          {ticks.map((tick) => <span key={tick.fraction} className="pointer-events-none absolute inset-y-0 w-px bg-line-soft" style={{ left: `${tick.fraction * 100}%` }} />)}
          <span className={`wf-bar absolute top-1/2 h-3 min-w-[3px] -translate-y-1/2 rounded-sm ${failed ? "bg-rose-500" : "bg-(--svc)"} ${selected === id ? "ring-2 ring-accent/30 ring-offset-2 ring-offset-surface-solid" : "opacity-80"}`} style={{ left: `${row.offset * 100}%`, width: `${row.width * 100}%` }} title={`${formatMs(row.startMs)} → ${formatMs(row.startMs + row.durationMs)}`} />
        </div>
        <span className={`text-right font-mono text-[11px] tabular-nums ${failed ? "font-semibold text-rose-600 dark:text-rose-300" : "text-muted"}`}>{formatMs(row.durationMs)}</span>
      </div>
    </li>;
  };
  return <section className="waterfall min-w-0 overflow-hidden rounded-2xl border border-line bg-surface-solid shadow-sm" aria-labelledby="waterfall-title">
    <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line-soft px-4 py-3.5 sm:px-5">
      <div className="flex items-center gap-2"><Layers3 size={16} className="text-muted" /><h2 id="waterfall-title" className="m-0 text-sm font-semibold">Span waterfall</h2><span className="rounded-md bg-neutral-bg px-1.5 py-0.5 text-[10px] font-medium text-muted">{model.rows.length.toLocaleString("en")} {model.rows.length === 1 ? "span" : "spans"}</span></div>
      <div className="flex min-w-0 flex-wrap items-center gap-2 max-sm:w-full">
        <label className="relative min-w-0 flex-[1_1_7rem] sm:w-44"><Search size={13} className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-faint" /><input ref={searchRef} type="search" aria-label="Search spans" placeholder="Search spans…" value={search} onChange={(event) => { resetScroll(); setSearch(event.target.value); }} className="h-9 w-full rounded-lg border border-glass-field-edge bg-glass-field py-1 pr-2 pl-8 text-xs text-text focus:border-accent focus:ring-2 focus:ring-accent/15" /></label>
        <div className="inline-flex rounded-lg border border-line p-0.5" role="group" aria-label="Span status filter">
          <button type="button" aria-pressed={!errorsOnly} onClick={() => { resetScroll(); setErrorsOnly(false); }} className={`min-h-7 whitespace-nowrap rounded-md px-2 py-1 text-[11px] ${!errorsOnly ? "bg-btn text-btn-text" : "text-muted"}`}>All spans</button>
          <button type="button" aria-pressed={errorsOnly} onClick={() => { resetScroll(); setErrorsOnly(true); }} className={`min-h-7 whitespace-nowrap rounded-md px-2 py-1 text-[11px] ${errorsOnly ? "bg-btn text-btn-text" : "text-muted"}`}>Errors <span className="ml-0.5 opacity-70">{model.errors}</span></button>
        </div>
      </div>
    </header>
    {selected && !selectedRow && <div className="flex flex-wrap items-center gap-2 border-b border-line-soft px-4 py-2 text-xs text-muted">
      <span role="status">The linked span is outside the spans loaded for this trace.</span>
      <button type="button" className="text-accent underline" onClick={closeInspector}>Clear span selection</button>
    </div>}
    <div className={`grid min-w-0 ${selectedRow && !useSheet ? "grid-cols-[minmax(0,1fr)_clamp(23rem,35%,30rem)]" : "grid-cols-1"}`}>
      <div className="@container/waterfall min-w-0">
        {(model.rows.some((row) => row.hasChildren) || focus || filtering) && <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 border-b border-line-soft px-4 py-1.5 text-xs text-muted">
          <span>{filtering ? `${rows.length} matching spans` : "Select a span to inspect its details"}</span>
          <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
            {focus && <button type="button" className="inline-flex min-w-0 items-center gap-1 text-left text-[11px] text-accent" onClick={() => onFocus("")}><span className="wrap-anywhere">Highlighting {focus}</span><X size={12} className="shrink-0" /></button>}
            {!filtering && model.rows.some((row) => row.hasChildren) && <><button type="button" className="min-h-7 whitespace-nowrap text-[11px] hover:text-text" onClick={() => { resetScroll(); setCollapsed(new Set(model.rows.filter((row) => row.hasChildren).map((row) => row.span.span_id))); }}>Collapse all</button><button type="button" className="min-h-7 whitespace-nowrap text-[11px] hover:text-text" onClick={() => setCollapsed(new Set())}>Expand all</button></>}
          </div>
        </div>}
        {model.orphans > 0 && <p role="note" className="m-0 border-b border-line-soft bg-amber-500/5 px-4 py-2 text-[11px] text-muted">{model.orphans} {model.orphans === 1 ? "span references a parent that was" : "spans reference parents that were"} not received, so the tree may be incomplete.</p>}
        <div className={`${columns} overflow-hidden border-b border-line-soft bg-surface-sunken/40 px-3 py-2 [scrollbar-gutter:stable] sm:px-4`}>
          <span className="text-[10px] font-semibold uppercase tracking-wide text-muted">Service / operation</span>
          <div className="wf-axis relative h-4 font-mono text-[10px] text-muted" aria-hidden="true">
            <span className="text-[10px] font-semibold uppercase tracking-wide @min-[30rem]/waterfall:hidden">Timeline</span>
            {ticks.map((tick) => <span key={tick.fraction} className={`absolute bottom-0 hidden whitespace-nowrap ${tick.fraction === 1 ? "-translate-x-full @min-[30rem]/waterfall:inline" : tick.fraction === 0 ? "@min-[30rem]/waterfall:inline" : "-translate-x-1/2 @min-[36rem]/waterfall:inline"}`} style={{ left: `${tick.fraction * 100}%` }}>{formatMs(tick.ms)}</span>)}
          </div>
          <span className="text-right text-[10px] font-semibold uppercase tracking-wide text-muted">Duration</span>
        </div>
        <div className="wf-viewport relative max-h-[min(35rem,55dvh)] overflow-y-auto [scrollbar-gutter:stable] max-md:max-h-[min(25rem,48dvh)]" ref={viewportRef}>
          {!model.rows.length ? <Empty>This trace has no spans.</Empty> : !rows.length ? <Empty>No spans match these filters.</Empty> : <ol className="wf-rows relative m-0 list-none p-0" style={isVirtualized ? { height: totalHeight } : undefined}>{isVirtualized ? virtualItems.map((item) => renderRow(rows[item.index], item.top)) : rows.map((row) => renderRow(row))}</ol>}
        </div>
        <footer className="flex flex-wrap items-center justify-between gap-2 border-t border-line-soft px-4 py-2.5 text-[10px] text-faint"><span>Time relative to the start of this trace</span><span className="inline-flex items-center gap-3"><span className="inline-flex items-center gap-1.5"><i className="size-1.5 rounded-full bg-accent" />By service</span><span className="inline-flex items-center gap-1.5"><i className="size-1.5 rounded-full bg-rose-500" />Error</span></span></footer>
      </div>
      {selectedRow && (useSheet
        ? <BottomSheet open title="Span details" onClose={closeInspector}>
          <div className="-mx-5"><SpanDetail key={selectedRow.span.span_id} span={selectedRow.span} traceID={traceID} startMs={selectedRow.startMs} durationMs={selectedRow.durationMs} /></div>
        </BottomSheet>
        : <aside className="wf-inspector max-h-[min(42rem,70dvh)] min-w-0 overflow-y-auto border-l border-line bg-surface-sunken/25" aria-label="Selected span inspector" onKeyDown={(event) => { if (event.key === "Escape") closeInspector(); }}>
          <SpanDetail key={selectedRow.span.span_id} span={selectedRow.span} traceID={traceID} startMs={selectedRow.startMs} durationMs={selectedRow.durationMs} onClose={closeInspector} />
        </aside>)}
    </div>
    {children}
  </section>;
}
