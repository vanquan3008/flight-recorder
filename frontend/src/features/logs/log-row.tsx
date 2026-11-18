import { ArrowUpRight, ChevronRight } from "lucide-react";
import { memo, useId, useMemo, useState } from "react";
import type { MouseEvent } from "react";
import type { LogRecord } from "../../shared/types";
import { Badge, CopyButton, DetailLink, formatTimeParts, logBodyText } from "../../shared/ui";
import { HttpStatusBadge, MethodBadge } from "./log-badges";
import { LogDetail } from "./log-detail";
import { normalizeSeverityText, parseHttpLog } from "./log-format";

export interface LogRowProps { log: LogRecord; forceOpen?: boolean }
const cell = "min-w-0 px-3 py-2.5 align-middle max-md:p-0";
export const requestColumns = "grid grid-cols-[44px_minmax(0,1fr)_40px_76px] items-center gap-2";
// Phone layout: [expand | service | time | severity] / [message] / [trace link · copy].
const mobileRow = "max-md:grid max-md:h-auto max-md:grid-cols-[30px_minmax(0,1fr)_auto_auto] max-md:items-center max-md:gap-x-2 max-md:gap-y-1.5 max-md:py-2.5 max-md:pr-3";
const place = {
  expand: "max-md:col-start-1 max-md:row-span-3 max-md:row-start-1 max-md:self-start max-md:py-0",
  service: "max-md:col-start-2 max-md:row-start-1",
  time: "max-md:col-start-3 max-md:row-start-1",
  severity: "max-md:col-start-4 max-md:row-start-1",
  body: "max-md:col-span-3 max-md:col-start-2 max-md:row-start-2",
  trace: "max-md:col-span-2 max-md:col-start-2 max-md:row-start-3",
  actions: "max-md:col-start-4 max-md:row-start-3 max-md:justify-self-end max-md:p-0",
};

export const LogRow = memo(function LogRow({ log, forceOpen }: LogRowProps) {
  const [userToggled, setUserToggled] = useState<boolean | null>(null);
  const isOpen = userToggled ?? Boolean(forceOpen);
  const detailID = useId();
  const body = useMemo(() => logBodyText(log.body), [log.body]);
  const http = useMemo(() => parseHttpLog(body), [body]);
  const severity = normalizeSeverityText(log.severity_text, log.severity_number);
  const timestamp = useMemo(() => formatTimeParts(log.occurred_at), [log.occurred_at]);
  const failed = severity === "ERROR" || severity === "FATAL";
  const rail = failed ? "border-l-rose-500" : severity === "WARN" ? "border-l-amber-500" : "border-l-transparent";
  const toggle = () => setUserToggled(!isOpen);
  const onRowClick = (event: MouseEvent<HTMLTableRowElement>) => {
    if (!(event.target as HTMLElement).closest("button, a, input, select, textarea")) toggle();
  };
  return <>
    <tr data-log-id={log.event_id} className={`log-table-row ${mobileRow} h-12 cursor-pointer border-b border-line-soft text-xs transition-colors ${isOpen ? "expanded-active bg-accent/5" : failed ? "bg-rose-500/3 hover:bg-rose-500/6" : "hover:bg-hover"}`}
      onClick={onRowClick} tabIndex={0} onKeyDown={(event) => {
        if (event.target !== event.currentTarget || !["Enter", " "].includes(event.key)) return;
        event.preventDefault(); toggle();
      }}>
      <td className={`border-l-2 px-1 py-2.5 align-middle ${rail} ${place.expand}`}>
        <button type="button" className="log-expand-icon-btn inline-flex size-7 items-center justify-center rounded-md text-muted hover:bg-hover hover:text-text" aria-expanded={isOpen} aria-controls={isOpen ? detailID : undefined}
          aria-label={isOpen ? "Collapse log details" : "Expand log details"} onClick={toggle}>
          <ChevronRight size={14} className={`transition-transform ${isOpen ? "rotate-90" : ""}`} />
        </button>
      </td>
      <td className={`${cell} log-col-time ${place.time}`}><time dateTime={log.occurred_at} title={timestamp?.full ?? log.occurred_at} className="block whitespace-nowrap font-mono text-[11px] text-muted tabular-nums">{timestamp?.time ?? log.occurred_at}</time></td>
      <td className={`${cell} log-col-service ${place.service}`}><span className="block truncate text-xs font-medium text-text" title={log.service_name}>{log.service_name}</span></td>
      <td className={`${cell} log-col-severity ${place.severity}`}><Badge value={severity} /></td>
      <td className={`${cell} log-col-body overflow-hidden ${place.body}`}>
        {http ? <div className={`log-http-preview ${requestColumns}`} title={http.firstLine}>
          <MethodBadge method={http.method} />
          <span className="log-path min-w-0 truncate font-mono text-xs text-text" title={http.path}>{http.path}</span>
          <HttpStatusBadge status={http.status} />
          <span className="log-duration whitespace-nowrap text-right font-mono text-[11px] text-muted tabular-nums">{http.duration}</span>
        </div> : <span className="log-message block truncate font-mono text-xs leading-5 text-text" title={body}>{body.length > 300 ? `${body.slice(0, 300)}…` : body}</span>}
      </td>
      <td className={`${cell} log-col-trace ${place.trace}`}>
        {log.trace_id ? <DetailLink to={`/traces/${log.trace_id}`} className="inline-flex items-center gap-1 whitespace-nowrap text-[11px] font-medium text-accent no-underline hover:underline">Open trace<ArrowUpRight size={12} aria-hidden="true" /></DetailLink>
          : <span className="text-faint" title="No trace linked to this log" aria-label="Not correlated">—</span>}
      </td>
      <td className={`log-col-actions px-3 py-2.5 text-center align-middle ${place.actions}`}><CopyButton value={body} label="Copy log body" compact /></td>
    </tr>
    {isOpen && <tr className="log-detail-row border-b border-line bg-surface-sunken max-md:block"><td id={detailID} colSpan={7} className="p-0 max-md:block"><LogDetail log={log} body={body} http={http} /></td></tr>}
  </>;
});
