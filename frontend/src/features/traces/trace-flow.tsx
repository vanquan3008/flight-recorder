import { AlertTriangle, ChevronDown, Clock3, GitBranch } from "lucide-react";
import type { CSSProperties } from "react";
import { useState } from "react";
import { ServiceGraph } from "../../shared/charts";
import { DataTable, formatDuration } from "../../shared/ui";
import { analyzeTraceInsights } from "./trace-format";
import { type TraceModel, formatMs } from "./trace-model";

type Props = { model: TraceModel; focus: string; onFocus: (service: string) => void };

export function TraceFlow({ model, focus, onFocus }: Props) {
  const [showMap, setShowMap] = useState(false);
  const [showAll, setShowAll] = useState(false);
  const selfTotal = Math.max(model.services.reduce((sum, service) => sum + service.selfMs, 0), 1);
  const insights = analyzeTraceInsights(model);
  const services = showAll ? model.services : model.services.slice(0, 8);
  return <section aria-labelledby="request-flow-title" className="overflow-hidden rounded-xl border border-line bg-surface-solid">
    <div className="flex flex-col items-stretch gap-x-5 gap-y-2 px-5 py-3 sm:flex-row sm:flex-wrap sm:items-center">
      <div className="flex shrink-0 items-center gap-2">
        <GitBranch size={15} className="text-faint" />
        <div><h2 id="request-flow-title" className="m-0 text-xs font-semibold">Request flow</h2><p className="m-0 mt-0.5 text-[10px] text-muted">{model.services.length === 1 ? "Single-service trace" : `${model.edges.length} service relationships`}</p></div>
      </div>
      <div className="flow-share min-w-0 flex-1">
        <h3 className="sr-only">Where the time went</h3>
        <ul className="m-0 flex list-none flex-wrap gap-1.5 p-0" aria-label="Self time per service, excluding time spent waiting on child spans">
          {services.map((service) => <li key={service.name} className={focus && focus !== service.name ? "opacity-40" : ""}>
            <button type="button" aria-pressed={focus === service.name} onClick={() => onFocus(focus === service.name ? "" : service.name)}
              title={`${service.name}: ${service.spans} ${service.spans === 1 ? "span" : "spans"}, ${service.errors} errors. Select to highlight spans.`}
              style={{ "--svc": `var(--svc-${service.index % 6})` } as CSSProperties}
              className={`inline-flex max-w-full items-center gap-2 rounded-md border border-solid px-2.5 py-1.5 text-[11px] transition-colors ${focus === service.name ? "border-accent bg-hover" : "border-line-soft hover:bg-hover"}`}>
              <i className="size-1.5 shrink-0 rounded-full bg-(--svc)" aria-hidden="true" /><span className="max-w-48 truncate font-medium">{service.name}</span><span className="whitespace-nowrap font-mono text-muted">{formatMs(service.selfMs)}</span><span className="text-faint">{Math.round(service.selfMs / selfTotal * 100)}%</span>
            </button>
          </li>)}
        </ul>
        {model.services.length > 8 && <button type="button" className="mt-2 text-[11px] text-accent" aria-expanded={showAll} onClick={() => setShowAll(!showAll)}>{showAll ? "Show fewer services" : `Show all ${model.services.length} services`}</button>}
      </div>
      {model.edges.length > 0 && <button type="button" className="inline-flex items-center gap-1.5 rounded-md border border-solid border-line px-2.5 py-1.5 text-[11px] text-muted hover:bg-hover" aria-expanded={showMap} aria-controls="trace-service-map" onClick={() => setShowMap(!showMap)}>Service map<ChevronDown size={12} className={showMap ? "rotate-180" : ""} /></button>}
    </div>
    {insights.length > 0 && <div role="region" aria-label="Trace diagnostic insights" className="divide-y divide-line-soft border-t border-line-soft">
      {insights.map((insight, index) => <div key={index} className={`flex gap-2.5 px-5 py-3 ${insight.type === "error" ? "bg-rose-500/5" : "bg-amber-500/5"}`}>
        {insight.type === "error" ? <AlertTriangle size={14} className="mt-0.5 shrink-0 text-rose-500" /> : <Clock3 size={14} className="mt-0.5 shrink-0 text-amber-500" />}
        <div className="min-w-0 text-xs"><strong className="font-semibold">{insight.title}</strong><p className="m-0 mt-1 text-[11px] leading-relaxed text-muted">{insight.description}</p></div>
      </div>)}
    </div>}
    {showMap && <div id="trace-service-map" className="border-t border-line-soft">
      <ServiceGraph nodes={model.nodes} edges={model.edges} selected={focus} onSelect={onFocus} label="Service flow for this trace" />
      <p className="m-0 px-5 py-3 text-[11px] text-muted">Arrows point from caller to callee. Select a service to highlight its spans.</p>
      <DataTable headers={["From", "To", "Calls", "Errors", "Avg", "Slowest"]} rows={model.edges.map((edge) => [edge.source_node_id, edge.target_node_id, edge.call_count, edge.error_count || "—", formatDuration(edge.average_latency_nanos), formatDuration(edge.p95_latency_nanos)])} />
    </div>}
  </section>;
}
