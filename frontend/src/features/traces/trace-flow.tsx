import { AlertTriangle, Clock } from "lucide-react";
import type { CSSProperties } from "react";
import { ServiceGraph } from "../../shared/charts";
import { Badge, DataTable, Panel, formatDuration } from "../../shared/ui";
import { analyzeTraceInsights } from "./trace-format";
import { type TraceModel, formatMs } from "./trace-model";

type Props = { model: TraceModel; focus: string; onFocus: (service: string) => void };

const swatch = (index: number): CSSProperties => ({ "--svc": `var(--svc-${index % 6})` } as CSSProperties);

/** Whole-request overview: totals, incident insights, who calls whom, and where the time went. */
export function TraceFlow({ model, focus, onFocus }: Props) {
  const selfTotal = Math.max(model.services.reduce((sum, service) => sum + service.selfMs, 0), 1);
  const insights = analyzeTraceInsights(model);

  const stats = [
    ["Total duration", formatMs(model.totalMs)],
    ["Spans", String(model.rows.length)],
    ["Services", String(model.services.length)],
    ["Errors", String(model.errors)],
  ];

  return (
    <Panel title="Request flow" meta="how this request moved through your services" className="trace-flow-panel">
      {/* KPI Stats Row */}
      <div className="flow-stats">
        {stats.map(([label, value]) => (
          <div key={label} className={label === "Errors" && model.errors ? "has-errors" : ""}>
            <span>{label}</span>
            <strong>{value}</strong>
          </div>
        ))}
      </div>

      {/* Smart Incident / Root Cause Insights */}
      {insights.length > 0 && (
        <div className="trace-insights-container" role="region" aria-label="Trace diagnostic insights">
          {insights.map((insight, idx) => (
            <div key={idx} className={`trace-insight-banner ${insight.type}`}>
              <div className="insight-icon" aria-hidden="true">
                {insight.type === "error" ? <AlertTriangle size={18} /> : <Clock size={18} />}
              </div>
              <div className="insight-body">
                <span className="insight-title">{insight.title}</span>
                <p className="insight-desc">{insight.description}</p>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Service Map or Single-Service Callout */}
      {model.edges.length ? (
        <div className="flow-graph-wrap">
          <ServiceGraph nodes={model.nodes} edges={model.edges} selected={focus} onSelect={onFocus} label="Service flow for this trace" />
          <p className="graph-legend muted">Arrows point from caller to callee. Select a service to highlight its spans in the waterfall below.</p>
        </div>
      ) : (
        <div className="flow-single-service">
          <p className="empty">
            Every span ran inside {model.services.length === 1 ? <strong>{model.services[0].name}</strong> : "one service"}, so there are no service-to-service calls to draw.
          </p>
        </div>
      )}

      {/* Time Share Distribution */}
      <div className="flow-share">
        <h3>
          Where the time went <span>self time per service, excluding time spent waiting on child spans</span>
        </h3>
        <ul>
          {model.services.map((service) => {
            const pct = Math.round((service.selfMs / selfTotal) * 100);
            return (
              <li key={service.name} style={swatch(service.index)} className={focus && focus !== service.name ? "dim" : ""}>
                <button type="button" aria-pressed={focus === service.name} onClick={() => onFocus(focus === service.name ? "" : service.name)}>
                  <span className="share-name">
                    <i aria-hidden="true" />
                    {service.name}
                  </span>
                  <span className="share-bar" aria-hidden="true">
                    <b style={{ width: `${Math.max((service.selfMs / selfTotal) * 100, 1)}%` }} />
                  </span>
                  <span className="share-value">
                    {formatMs(service.selfMs)} · {pct}%
                  </span>
                  <span className="share-meta">
                    {service.spans} {service.spans === 1 ? "span" : "spans"}
                    {service.errors > 0 && (
                      <>
                        {" · "}
                        <Badge value="ERROR" /> {service.errors}
                      </>
                    )}
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      </div>

      {model.edges.length > 0 && (
        <DataTable
          headers={["From", "To", "Calls", "Errors", "Avg", "Slowest"]}
          rows={model.edges.map((edge) => [
            edge.source_node_id,
            edge.target_node_id,
            edge.call_count,
            edge.error_count || "—",
            formatDuration(edge.average_latency_nanos),
            formatDuration(edge.p95_latency_nanos),
          ])}
        />
      )}
    </Panel>
  );
}
