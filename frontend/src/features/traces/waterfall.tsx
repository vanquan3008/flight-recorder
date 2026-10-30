import { ChevronDown, ChevronRight, CircleAlert } from "lucide-react";
import type { CSSProperties } from "react";
import { useMemo, useState } from "react";
import { Empty } from "../../shared/ui";
import { SpanDetail } from "./span-detail";
import { type TraceModel, axisTicks, formatMs, isError, visibleRows } from "./trace-model";

type Props = { model: TraceModel; traceID: string; focus: string; onFocus: (service: string) => void };

const swatch = (index: number): CSSProperties => ({ "--svc": `var(--svc-${index % 6})` } as CSSProperties);

export function Waterfall({ model, traceID, focus, onFocus }: Props) {
  const [selected, setSelected] = useState("");
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const rows = useMemo(() => visibleRows(model.rows, collapsed), [model.rows, collapsed]);
  const ticks = useMemo(() => axisTicks(model.totalMs), [model.totalMs]);
  if (!model.rows.length) return <Empty>This trace has no spans.</Empty>;
  const parents = model.rows.filter((row) => row.hasChildren).map((row) => row.span.span_id);
  const toggle = (id: string) => setCollapsed((current) => {
    const next = new Set(current);
    if (!next.delete(id)) next.add(id);
    return next;
  });
  return <div className="waterfall">
    <div className="wf-toolbar">
      <div className="wf-legend" role="group" aria-label="Filter spans by service">
        {model.services.map((service) => <button type="button" className="chip wf-chip" style={swatch(service.index)} aria-pressed={focus === service.name} key={service.name} onClick={() => onFocus(focus === service.name ? "" : service.name)}>
          <i aria-hidden="true" />{service.name}<small>{service.spans}</small>
        </button>)}
      </div>
      {parents.length > 0 && <div className="wf-actions">
        <button type="button" className="button-secondary compact" onClick={() => setCollapsed(new Set(parents))}>Collapse all</button>
        <button type="button" className="button-secondary compact" onClick={() => setCollapsed(new Set())}>Expand all</button>
      </div>}
    </div>
    {model.orphans > 0 && <p className="muted wf-note" role="note">{model.orphans === 1 ? "1 span references a parent that was" : `${model.orphans} spans reference parents that were`} not received, so the tree may be incomplete.</p>}
    <div className="wf-axis" aria-hidden="true">{ticks.map((tick) => <span key={tick.fraction} style={{ left: `${tick.fraction * 100}%` }} className={tick.fraction === 1 ? "end" : ""}>{formatMs(tick.ms)}</span>)}</div>
    <ol className="wf-rows">{rows.map((row) => {
      const id = row.span.span_id;
      const failed = isError(row.span);
      const dim = Boolean(focus) && row.span.service_name !== focus;
      return <li className={`wf-row${dim ? " dim" : ""}${selected === id ? " selected" : ""}${failed ? " failed" : ""}`} style={swatch(row.serviceIndex)} key={id}>
        <div className="wf-head" style={{ paddingLeft: `${row.depth * 20}px` }}>
          {row.hasChildren
            ? <button type="button" className="wf-toggle" aria-expanded={!collapsed.has(id)} aria-label={`${collapsed.has(id) ? "Expand" : "Collapse"} ${row.span.name}`} onClick={() => toggle(id)}>{collapsed.has(id) ? <ChevronRight size={14} /> : <ChevronDown size={14} />}</button>
            : <span className="wf-toggle spacer" />}
          <button type="button" className="wf-main" aria-pressed={selected === id} onClick={() => setSelected(selected === id ? "" : id)}>
            <span className="wf-name">{row.span.name}</span>
            <span className="wf-service">{row.span.service_name}</span>
            {failed && <span className="wf-error"><CircleAlert size={13} aria-hidden="true" /> Error</span>}
            <span className="wf-duration">{formatMs(row.durationMs)}</span>
          </button>
        </div>
        <div className="wf-rail" aria-hidden="true"><span className="wf-bar" style={{ left: `${row.offset * 100}%`, width: `${row.width * 100}%` }} /></div>
        {selected === id && <SpanDetail span={row.span} traceID={traceID} startMs={row.startMs} durationMs={row.durationMs} />}
      </li>;
    })}</ol>
  </div>;
}
