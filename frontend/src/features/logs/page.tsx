import { Radio, Search, X } from "lucide-react";
import type { FormEvent } from "react";
import { useCallback, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useAuth } from "../../app/auth";
import { compactSearchParams, pageItems, queryString, useLiveStream, usePagedQuery } from "../../shared/api";
import type { LogRecord } from "../../shared/types";
import { Badge, DetailLink, ExpandableText, PageHeader, PagedTable, Panel, TimeWindowFilter, formatTime, logBodyText, useTimeWindow } from "../../shared/ui";
import "./logs.css";

const severities = ["TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"];
const filterNames = ["service", "severity", "search", "traceId", "spanId"];

export function LogsPage() {
  const { credential } = useAuth();
  const [params, setParams] = useSearchParams();
  const window = useTimeWindow();
  const [liveTail, setLiveTail] = useState(false);
  const [liveLogs, setLiveLogs] = useState<LogRecord[]>([]);

  const onLiveLog = useCallback((newLog: LogRecord) => {
    const serviceFilter = params.get("service");
    const severityFilter = params.get("severity");
    if (serviceFilter && newLog.service_name !== serviceFilter) return;
    if (severityFilter && newLog.severity_text !== severityFilter) return;
    setLiveLogs((prev) => [newLog, ...prev.filter((item) => item.event_id !== newLog.event_id)].slice(0, 100));
  }, [params]);

  const { connected } = useLiveStream({
    enabled: liveTail,
    credential,
    onLog: onLiveLog,
  });

  const traceID = params.get("traceId") || "";
  const anyTime = Boolean(traceID) && (!params.has("range") || params.get("range") === "any");
  const search = params.get("search") || "";
  const longWindow = !anyTime && window.range === "7d";
  const values = Object.fromEntries(filterNames.map((name) => [name, params.get(name) || undefined]));
  const path = `/api/v1/logs?${queryString({ ...values, from: anyTime ? undefined : window.from, to: anyTime ? undefined : window.to })}`;
  const query = usePagedQuery<LogRecord>(["logs"], path, credential);
  const pagedLogs = pageItems(query.data?.pages);
  const logs = liveTail && liveLogs.length > 0 ? [...liveLogs, ...pagedLogs] : pagedLogs;
  const active = filterNames.filter((name) => params.get(name));

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setParams(compactSearchParams(event.currentTarget, params, ["range", "to"]));
  };
  const pick = (severity: string) => {
    const next = new URLSearchParams(params);
    if (severity) next.set("severity", severity); else next.delete("severity");
    setParams(next);
  };
  const clear = () => setParams(new URLSearchParams([...params].filter(([key]) => !filterNames.includes(key))));

  return <>
    <PageHeader
      title="Log explorer"
      action={
        <div className="header-controls log-header-controls">
          <button
            type="button"
            className={`button-secondary compact live-tail-btn${liveTail ? " active" : ""}`}
            aria-pressed={liveTail}
            onClick={() => {
              setLiveTail(!liveTail);
              if (liveTail) setLiveLogs([]);
            }}
          >
            <span className={`live-dot${connected ? " connected" : ""}`} />
            <Radio size={13} aria-hidden="true" />
            {liveTail ? (connected ? "Live streaming" : "Connecting...") : "Live tail"}
          </button>
          <TimeWindowFilter range={anyTime ? "any" : window.range} select={window.select} allowAny={Boolean(traceID)} />
        </div>
      }
    >
      Read logs on their own or follow a log to the trace and span that produced it.
    </PageHeader>
    <div className="log-toolbar-bar">
      <div className="severity-chips" role="group" aria-label="Quick severity filter">
        {["", ...severities.slice(2)].map((severity) => (
          <button
            type="button"
            key={severity || "all"}
            className={`severity-chip ${severity}`}
            aria-pressed={(params.get("severity") || "") === severity}
            onClick={() => pick(severity)}
          >
            {severity || "All"}
          </button>
        ))}
      </div>
      <form className="toolbar log-filters" key={filterNames.map((name) => params.get(name) || "").join("|")} onSubmit={submit}>
        <input name="service" placeholder="Service" aria-label="Filter logs by service" defaultValue={params.get("service") || ""} />
        <input type="hidden" name="severity" value={params.get("severity") || ""} />
        <input name="search" placeholder="Search body text" aria-label="Search log body" defaultValue={search} />
        <input name="traceId" placeholder="Trace ID" aria-label="Filter logs by trace ID" defaultValue={traceID} />
        <input name="spanId" placeholder="Span ID" aria-label="Filter logs by span ID" defaultValue={params.get("spanId") || ""} />
        <button type="submit"><Search size={15} /> Search logs</button>
        {active.length > 0 && <button type="button" className="button-secondary" onClick={clear}><X size={15} /> Clear filters</button>}
      </form>
    </div>
    {search && (anyTime || longWindow) && <p className="muted log-note" role="note">Body search needs a time range of 24 hours or less. Choose a shorter range if the request is rejected.</p>}
    <Panel className="log-panel" title="Logs" meta={query.isPending ? undefined : `${logs.length} loaded${liveTail && liveLogs.length > 0 ? ` (${liveLogs.length} live)` : ""}${query.hasNextPage ? " · more available" : ""}`}>
      <PagedTable query={query} empty="No logs match these filters in the selected time range. Widen the range or clear filters."
        headers={["Time", "Service", "Severity", "Body", "Trace"]}
        rows={logs.map((log) => [
          formatTime(log.occurred_at), log.service_name, <Badge value={log.severity_text} />,
          <ExpandableText value={logBodyText(log.body)} />,
          log.trace_id ? <DetailLink to={`/traces/${log.trace_id}`}>Open trace</DetailLink> : <span className="muted">Not correlated</span>,
        ])} />
    </Panel>
  </>;
}
