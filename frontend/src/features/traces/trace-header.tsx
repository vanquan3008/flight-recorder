import { Activity, ArrowUpRight, CheckCircle2, CircleAlert, Clock3, Layers3, Network } from "lucide-react";
import type { TraceModel } from "./trace-model";
import { formatMs } from "./trace-model";
import { getSpanDisplayInfo } from "./trace-format";
import { BackLink, CopyButton, DetailLink, formatTime } from "../../shared/ui";

export function TraceHeader({ model, traceID }: { model: TraceModel; traceID: string }) {
  const root = model.rows[0]?.span;
  const info = root ? getSpanDisplayInfo(root) : undefined;
  const stats = [
    { label: "Total duration", value: model.rows.length ? formatMs(model.totalMs) : "—", Icon: Clock3 },
    { label: "Spans", value: model.rows.length.toLocaleString("en"), Icon: Layers3 },
    { label: "Services", value: model.services.length.toLocaleString("en"), Icon: Network },
    { label: "Errors", value: String(model.errors), Icon: CircleAlert },
  ];
  return <header className="min-w-0">
    <div className="mb-4 flex flex-wrap items-center justify-between gap-3 text-xs text-muted max-md:mb-3">
      <div className="[&_.back-link]:inline-flex [&_.back-link]:min-h-9 [&_.back-link]:items-center [&_.back-link]:gap-1.5"><BackLink fallback="/traces" fallbackLabel="traces" /></div>
      <div className="flex min-w-0 max-w-full items-center gap-2"><span className="shrink-0 text-[10px] font-semibold uppercase tracking-wider">Trace ID</span><code className="min-w-0 truncate text-[11px] text-muted">{traceID}</code><CopyButton value={traceID} label="Copy trace ID" /></div>
    </div>
    <div className="overflow-hidden rounded-2xl border border-line bg-surface-solid shadow-sm">
      <div className="flex flex-wrap items-start justify-between gap-4 px-5 py-5 sm:px-6 max-md:gap-3 max-md:px-4 max-md:py-4">
        {/* Wrap the status badges below the title before the title gets narrower than 18rem. */}
        <div className="min-w-0 flex-[1_1_18rem]">
          <div className="mb-2.5 flex flex-wrap items-center gap-2 text-xs text-muted">
            <span className="inline-flex items-center gap-1.5 font-semibold text-text"><Activity size={14} className="text-accent" />Trace detail</span>
            <span aria-hidden="true">/</span><span>{root?.service_name || "No service recorded"}</span>
            {root && <><span aria-hidden="true">·</span><time dateTime={root.started_at}>{formatTime(root.started_at)}</time></>}
          </div>
          <h1 className="m-0 break-words text-xl font-semibold tracking-tight text-text sm:text-2xl">{info?.displayName || "Trace detail"}</h1>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <span className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium ${model.errors ? "bg-rose-500/10 text-rose-700 dark:text-rose-300" : "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"}`}>
            {model.errors ? <CircleAlert size={13} /> : <CheckCircle2 size={13} />}{model.errors ? "Errors recorded" : info?.httpStatus ? `HTTP ${info.httpStatus}` : "No errors recorded"}
          </span>
          <DetailLink to={`/logs?traceId=${traceID}`} className="inline-flex items-center gap-1.5 rounded-lg border border-line px-3 py-1.5 text-xs font-medium text-muted no-underline hover:bg-hover hover:text-text">Open logs<ArrowUpRight size={13} /></DetailLink>
        </div>
      </div>
      <div className="flow-stats grid grid-cols-2 border-t border-line-soft bg-surface-sunken/30 sm:grid-cols-4">
        {stats.map(({ label, value, Icon }) => <div key={label} className="flex items-center justify-between gap-3 border-r border-line-soft px-5 py-3.5 last:border-r-0 sm:px-6">
          <div><span className="mb-1 block text-[11px] text-muted">{label}</span><strong className={`text-xl font-semibold tabular-nums tracking-tight ${label === "Errors" && model.errors ? "text-rose-600 dark:text-rose-400" : "text-text"}`}>{value}</strong></div>
          <Icon size={17} className="shrink-0 text-faint" strokeWidth={1.6} aria-hidden="true" />
        </div>)}
      </div>
    </div>
  </header>;
}
