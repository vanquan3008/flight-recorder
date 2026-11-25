import { ExternalLink, Globe, Tag } from "lucide-react";
import type { LogRecord } from "../../shared/types";
import { traceURL } from "../../shared/ui/links";
import { CopyButton, DetailLink, formatTime } from "../../shared/ui";
import { HttpStatusBadge, MethodBadge } from "./log-badges";
import type { ParsedHttpLog } from "./log-format";

const sectionTitle = "mb-2 text-[10px] font-semibold uppercase tracking-wider text-muted";

export function LogDetail({ log, body, http }: { log: LogRecord; body: string; http: ParsedHttpLog | null }) {
  const dropped = log.dropped_attributes_count ?? 0;
  return <div className="log-detail-card min-w-0 space-y-4 px-5 py-5 sm:px-8">
    <div className="flex flex-wrap items-start justify-between gap-3 border-b border-line pb-3">
      <div className="min-w-0 text-xs"><strong className="font-semibold">{log.service_name}</strong><p className="mt-1 text-[11px] text-muted">{formatTime(log.occurred_at)}</p><p className="mt-1 break-all font-mono text-[10px] text-faint">Event {log.event_id}</p></div>
      <div className="flex flex-wrap gap-2 text-[11px] font-medium text-accent">
        {log.trace_id && <DetailLink to={traceURL(log.trace_id, log.span_id)} className="inline-flex items-center gap-1 rounded-md border border-line bg-surface-solid px-2.5 py-1.5 no-underline hover:bg-hover"><ExternalLink size={12} />Open trace</DetailLink>}
        {log.span_id && <DetailLink to={`/logs?traceId=${log.trace_id ?? ""}&spanId=${log.span_id}`} className="inline-flex items-center gap-1 rounded-md border border-line bg-surface-solid px-2.5 py-1.5 no-underline hover:bg-hover"><Tag size={12} />Logs for this span</DetailLink>}
      </div>
    </div>
    {http && <section aria-label="HTTP request details" className="space-y-3">
      <h3 className={`${sectionTitle} flex items-center gap-1.5`}><Globe size={12} />HTTP request</h3>
      <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Fact label="Method"><MethodBadge method={http.method} /></Fact>
        <Fact label="Status"><HttpStatusBadge status={http.status} /></Fact>
        <Fact label="Duration"><span className="font-mono text-xs tabular-nums">{http.duration}</span></Fact>
        {http.remoteIp && <Fact label="Remote IP"><code className="break-all text-xs text-text">{http.remoteIp}</code></Fact>}
      </dl>
      <div className="flex items-start justify-between gap-3 rounded-lg border border-line bg-surface-solid p-3"><code className="min-w-0 break-all text-xs leading-relaxed text-text">{http.path}</code><CopyButton value={http.path} label="Copy path" compact /></div>
      {http.headers.length > 0 && <details className="rounded-lg border border-line bg-surface-solid p-3">
        <summary className="cursor-pointer text-xs font-medium">Request headers ({http.headers.length})</summary>
        <dl className="mt-3 grid max-h-56 gap-2 overflow-auto">{http.headers.map((header, index) => <div key={`${header.name}:${index}`} className="grid grid-cols-[minmax(120px,25%)_minmax(0,1fr)] gap-3 text-[11px]"><dt className="break-words font-medium text-muted">{header.name}</dt><dd className="m-0 break-all font-mono">{header.value}</dd></div>)}</dl>
      </details>}
    </section>}
    <section aria-label="Full log body">
      <div className="mb-2 flex items-center justify-between gap-3"><h3 className={`${sectionTitle} m-0`}>Full log body <span className="ml-1 font-normal normal-case tracking-normal text-faint">{body.length.toLocaleString("en")} characters</span></h3><CopyButton value={body} label="Copy full log body" compact /></div>
      <pre className="m-0 max-h-72 overflow-auto rounded-lg border border-line bg-surface-solid p-3 font-mono text-xs leading-relaxed break-words whitespace-pre-wrap text-text">{body}</pre>
    </section>
    {dropped > 0 && <p role="note" className="m-0 rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-[11px] text-muted">The SDK dropped {dropped} {dropped === 1 ? "attribute" : "attributes"} before export.</p>}
    <AttributeList title="Log attributes" entries={objectEntries(log.attributes)} open />
    <AttributeList title="Resource attributes" entries={objectEntries(log.resource_attributes)} />
  </div>;
}

const objectEntries = (value: unknown): [string, unknown][] => value && typeof value === "object" && !Array.isArray(value) ? Object.entries(value) : [];

function AttributeList({ title, entries, open = false }: { title: string; entries: [string, unknown][]; open?: boolean }) {
  if (!entries.length) return null;
  return <details open={open} className="rounded-lg border border-line bg-surface-solid p-3">
    <summary className="cursor-pointer text-xs font-medium">{title} ({entries.length})</summary>
    <dl className="mt-3 grid max-h-56 gap-2 overflow-auto">{entries.map(([key, value]) => <div key={key} className="grid grid-cols-[minmax(120px,25%)_minmax(0,1fr)] gap-3 text-[11px]"><dt className="break-words font-medium text-muted">{key}</dt><dd className="m-0 break-all font-mono">{typeof value === "object" ? JSON.stringify(value) : String(value)}</dd></div>)}</dl>
  </details>;
}

function Fact({ label, children }: { label: string; children: React.ReactNode }) {
  return <div className="min-w-0 rounded-lg border border-line bg-surface-solid p-3"><dt className="mb-2 text-[10px] text-muted">{label}</dt><dd className="m-0">{children}</dd></div>;
}
