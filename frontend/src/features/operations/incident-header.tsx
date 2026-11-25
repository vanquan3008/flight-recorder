import { Activity, ArrowUpRight, Clock3, Hash, Layers3, Server } from "lucide-react";
import type { Incident } from "../../shared/types";
import { BackLink, Badge, CopyButton, DetailLink, formatTime } from "../../shared/ui";
import { secondaryButtonClass } from "../../shared/ui/styles";

const incidentTransitions: Record<string, string[]> = {
  OPEN: ["INVESTIGATING", "RESOLVED", "IGNORED"], INVESTIGATING: ["RESOLVED", "IGNORED"],
  RESOLVED: ["OPEN"], IGNORED: ["OPEN"],
};
const actions: Record<string, string> = { INVESTIGATING: "Start investigating", RESOLVED: "Resolve incident", IGNORED: "Ignore incident", OPEN: "Reopen incident" };

export function IncidentHeader({ incident, pending, error, onStatus }: { incident: Incident; pending: boolean; error?: Error | null; onStatus: (status: string) => void }) {
  return <header className="min-w-0">
    <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
      <BackLink fallback="/incidents" fallbackLabel="incidents" />
      <div className="flex min-w-0 items-center gap-2 text-[11px] text-muted"><Hash size={12} /><code className="truncate">{incident.id}</code><CopyButton value={incident.id} label="Copy incident ID" /></div>
    </div>
    <section aria-label="Incident summary" className="overflow-hidden rounded-2xl border border-line bg-surface-solid shadow-sm">
      <div className="flex flex-wrap items-start justify-between gap-4 px-5 py-5 sm:px-6">
        <div className="min-w-0 flex-1">
          <div className="mb-2 flex flex-wrap items-center gap-2 text-xs text-muted"><Activity size={14} className="text-accent" /><span className="font-medium">Incident investigation</span><span aria-hidden="true">/</span><span className="inline-flex items-center gap-1.5"><Server size={12} />{incident.primary_service}</span></div>
          <h1 className="m-0 break-words text-xl leading-snug font-semibold tracking-tight sm:text-2xl">{incident.title}</h1>
          {incident.error_message && incident.error_message !== incident.title && <p className="mt-2 max-w-3xl break-words text-[13px] leading-relaxed text-muted">{incident.error_message}</p>}
        </div>
        <div className="flex flex-wrap items-center gap-2" aria-live="polite"><Badge value={incident.status} /><Badge value={incident.severity} /></div>
      </div>
      <dl className="m-0 grid grid-cols-1 divide-y divide-line-soft border-y border-line-soft bg-surface-sunken/30 sm:grid-cols-3 sm:divide-x sm:divide-y-0">
        <div className="px-5 py-3.5"><dt className="mb-1 flex items-center gap-1.5 text-[11px] text-muted"><Layers3 size={12} />Total occurrences</dt><dd className="m-0 text-xl font-semibold tabular-nums">{incident.occurrence_count.toLocaleString("en")}</dd></div>
        <div className="px-5 py-3.5"><dt className="mb-1 flex items-center gap-1.5 text-[11px] text-muted"><Clock3 size={12} />First seen</dt><dd className="m-0 text-xs leading-relaxed"><time dateTime={incident.first_seen_at}>{formatTime(incident.first_seen_at)}</time></dd></div>
        <div className="px-5 py-3.5"><dt className="mb-1 flex items-center gap-1.5 text-[11px] text-muted"><Clock3 size={12} />Last seen</dt><dd className="m-0 text-xs leading-relaxed"><time dateTime={incident.last_seen_at}>{formatTime(incident.last_seen_at)}</time></dd></div>
      </dl>
      <div className="flex flex-wrap items-center justify-between gap-3 px-5 py-3">
        <div className="flex flex-wrap items-center gap-2" aria-label="Incident status actions">
          {(incidentTransitions[incident.status] ?? []).map((status) => <button key={status} type="button" disabled={pending} onClick={() => onStatus(status)}
            className={`${secondaryButtonClass} ${status === "RESOLVED" ? "border-emerald-500/25 text-ok-fg" : ""}`}>{actions[status]}</button>)}
          {pending && <span role="status" className="text-xs text-muted">Updating status…</span>}
        </div>
        {incident.trace_id && <DetailLink to={`/traces/${incident.trace_id}`} className="inline-flex items-center gap-1.5 text-xs font-medium text-accent no-underline hover:underline">Open linked trace<ArrowUpRight size={13} /></DetailLink>}
      </div>
      {error && <p role="alert" className="m-0 border-t border-rose-500/20 bg-danger-bg px-5 py-3 text-xs text-danger-fg">{error.message}</p>}
    </section>
  </header>;
}
