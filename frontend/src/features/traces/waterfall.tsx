import { ChevronDown, ChevronRight, CircleAlert } from "lucide-react";
import type { CSSProperties } from "react";
import { useCallback, useMemo, useRef, useState } from "react";
import { Empty } from "../../shared/ui";
import { SpanDetail } from "./span-detail";
import { getSpanDisplayInfo } from "./trace-format";
import { type TraceModel, axisTicks, formatMs, isError, visibleRows } from "./trace-model";
import { useVirtualList } from "./use-virtual-list";

type Props = { model: TraceModel; traceID: string; focus: string; onFocus: (service: string) => void };

const swatch = (index: number): CSSProperties => ({ "--svc": `var(--svc-${index % 6})` } as CSSProperties);

const ROW_HEIGHT = 48;
const DETAIL_HEIGHT = 440;

export function Waterfall({ model, traceID, focus, onFocus }: Props) {
  const [selected, setSelected] = useState("");
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const viewportRef = useRef<HTMLDivElement>(null);

  const rows = useMemo(() => visibleRows(model.rows, collapsed), [model.rows, collapsed]);
  const ticks = useMemo(() => axisTicks(model.totalMs), [model.totalMs]);

  const p95 = useMemo(() => {
    const durations = model.rows.map((r) => r.durationMs).sort((a, b) => a - b);
    if (!durations.length) return 0;
    return durations[Math.floor(durations.length * 0.95)] || 0;
  }, [model.rows]);

  const estimateHeight = useCallback((index: number) => {
    return rows[index]?.span.span_id === selected ? ROW_HEIGHT + DETAIL_HEIGHT : ROW_HEIGHT;
  }, [rows, selected]);

  const { virtualItems, totalHeight, isVirtualized } = useVirtualList({
    count: rows.length,
    estimateHeight,
    containerRef: viewportRef,
    overscan: 10,
  });

  if (!model.rows.length) return <Empty>This trace has no spans.</Empty>;
  const parents = model.rows.filter((row) => row.hasChildren).map((row) => row.span.span_id);
  const toggle = (id: string) => setCollapsed((current) => {
    const next = new Set(current);
    if (!next.delete(id)) next.add(id);
    return next;
  });

  const renderRow = (row: typeof rows[0], top?: number) => {
    const id = row.span.span_id;
    const failed = isError(row.span);
    const dim = Boolean(focus) && row.span.service_name !== focus;
    const info = getSpanDisplayInfo(row.span);
    const isSlow = !failed && model.rows.length > 2 && p95 > 50 && row.durationMs >= p95;
    const barTone = failed ? "error" : isSlow ? "slow" : "success";

    const resAttrs = (row.span.resource_attributes || {}) as Record<string, unknown>;
    const spanAttrs = (row.span.attributes || {}) as Record<string, unknown>;
    const envOrRegion = String(
      resAttrs["cloud.region"] || spanAttrs["cloud.region"] || resAttrs["deployment.environment"] || ""
    );

    const rowStyle: CSSProperties = {
      ...swatch(row.serviceIndex),
      ...(top !== undefined ? { position: "absolute", top: 0, left: 0, right: 0, transform: `translateY(${top}px)` } : {}),
    };

    return (
      <li className={`wf-row${dim ? " dim" : ""}${selected === id ? " selected" : ""}${failed ? " failed" : ""}`} style={rowStyle} key={id}>
        <div className="wf-row-grid">
          {/* Left Column: 2-Line Hierarchy & Span Info */}
          <div className="wf-head" style={{ paddingLeft: `${row.depth * 18}px` }}>
            {row.hasChildren ? (
              <button
                type="button"
                className="wf-toggle"
                aria-expanded={!collapsed.has(id)}
                aria-label={`${collapsed.has(id) ? "Expand" : "Collapse"} ${row.span.name}`}
                onClick={() => toggle(id)}
              >
                {collapsed.has(id) ? <ChevronRight size={14} /> : <ChevronDown size={14} />}
              </button>
            ) : (
              <span className="wf-toggle spacer" />
            )}

            <button
              type="button"
              className="wf-main"
              aria-pressed={selected === id}
              onClick={() => setSelected(selected === id ? "" : id)}
            >
              <div className="wf-main-content">
                <div className="wf-service-row">
                  <span className="wf-service" style={{ color: "var(--svc)" }}>
                    {row.span.service_name}
                  </span>
                  {failed && (
                    <span className="wf-error-badge">
                      <span className="wf-error">
                        <CircleAlert size={11} aria-hidden="true" /> Error
                      </span>
                      {info.httpStatus && <span className="wf-status-code">{info.httpStatus}</span>}
                    </span>
                  )}
                  {envOrRegion && <span className="wf-env-pill">{envOrRegion}</span>}
                </div>
                <div className="wf-op-row">
                  {info.method && !info.displayName.startsWith(info.method) && (
                    <span className={`wf-method-badge ${info.method.toLowerCase()}`}>
                      {info.method}
                    </span>
                  )}
                  <span className="wf-name" title={info.displayName}>
                    {info.displayName}
                  </span>
                </div>
              </div>
            </button>
          </div>

          {/* Right Column: Unified Timeline Rail */}
          <div
            className="wf-rail"
            aria-hidden="true"
            onClick={() => setSelected(selected === id ? "" : id)}
          >
            {/* Guide line markers matching ticks */}
            <div className="wf-rail-guides">
              {ticks.map((tick) => (
                <span key={tick.fraction} style={{ left: `${tick.fraction * 100}%` }} />
              ))}
            </div>

            {/* Start timestamp label */}
            {row.offset > 0.08 && (
              <span
                className="wf-time-start"
                style={{ left: `calc(${row.offset * 100}% - 4px)` }}
              >
                {formatMs(row.startMs)}
              </span>
            )}

            <span
              className={`wf-bar ${barTone}`}
              style={{ left: `${row.offset * 100}%`, width: `${row.width * 100}%` }}
            >
              {row.width >= 0.16 && (
                <span className="wf-bar-inner-label">{formatMs(row.durationMs)}</span>
              )}
            </span>

            {/* End timestamp label */}
            <span
              className="wf-time-end"
              style={{
                left: `calc(${row.offset * 100}% + ${Math.max(row.width * 100, 1.2)}% + 6px)`,
              }}
            >
              {formatMs(row.durationMs)}
              {failed && <span className="wf-bar-err-icon">!</span>}
            </span>
          </div>
        </div>

        {/* Selected Detail Inspector */}
        {selected === id && (
          <div className="wf-detail-wrap">
            <SpanDetail
              span={row.span}
              traceID={traceID}
              startMs={row.startMs}
              durationMs={row.durationMs}
              onClose={() => setSelected("")}
            />
          </div>
        )}
      </li>
    );
  };

  return (
    <div className="waterfall">
      {/* Legend & Filter Bar */}
      <div className="wf-toolbar">
        <div className="wf-title-group">
          <span className="wf-main-title">Distributed Trace Waterfall</span>
          <span className="wf-spans-badge">{model.rows.length} SPANS</span>
        </div>

        <div className="wf-legend" role="group" aria-label="Filter spans by service">
          {model.services.map((service) => (
            <button
              type="button"
              className="chip wf-chip"
              style={swatch(service.index)}
              aria-pressed={focus === service.name}
              key={service.name}
              onClick={() => onFocus(focus === service.name ? "" : service.name)}
            >
              <i aria-hidden="true" />
              {service.name}
              <small>{service.spans}</small>
            </button>
          ))}
        </div>

        {parents.length > 0 && (
          <div className="wf-actions">
            <button type="button" className="button-secondary compact" onClick={() => setCollapsed(new Set(parents))}>
              Collapse all
            </button>
            <button type="button" className="button-secondary compact" onClick={() => setCollapsed(new Set())}>
              Expand all
            </button>
          </div>
        )}
      </div>

      {model.orphans > 0 && (
        <p className="muted wf-note" role="note">
          {model.orphans === 1 ? "1 span references a parent that was" : `${model.orphans} spans reference parents that were`} not received, so the tree may be incomplete.
        </p>
      )}

      {/* Synchronized Header: Operation Column & Timeline Axis */}
      <div className="wf-header-grid">
        <div className="wf-col-label">
          <span>Service / Operation</span>
        </div>
        <div className="wf-axis" aria-hidden="true">
          {ticks.map((tick) => (
            <span
              key={tick.fraction}
              style={{ left: `${tick.fraction * 100}%` }}
              className={tick.fraction === 1 ? "end" : tick.fraction === 0 ? "start" : ""}
            >
              {formatMs(tick.ms)}
            </span>
          ))}
        </div>
      </div>

      {/* Waterfall Rows List */}
      {isVirtualized ? (
        <div className="wf-viewport" ref={viewportRef} style={{ maxHeight: "72vh", overflowY: "auto", position: "relative" }}>
          <ol className="wf-rows" style={{ height: `${totalHeight}px`, position: "relative" }}>
            {virtualItems.map((item) => renderRow(rows[item.index], item.top))}
          </ol>
        </div>
      ) : (
        <ol className="wf-rows">
          {rows.map((row) => renderRow(row))}
        </ol>
      )}

      {/* Bottom Status Legend matching standard APM command center */}
      <div className="wf-bottom-legend" aria-label="Waterfall status legend">
        <span className="legend-item success">
          <span className="legend-swatch success" />
          Successful
        </span>
        <span className="legend-item slow">
          <span className="legend-swatch slow" />
          Slow (&gt;p95)
        </span>
        <span className="legend-item error">
          <span className="legend-swatch error" />
          Error
        </span>
      </div>
    </div>
  );
}
