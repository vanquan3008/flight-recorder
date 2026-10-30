import { Search, X } from "lucide-react";
import type { FormEvent } from "react";
import { useSearchParams } from "react-router-dom";
import { useAuth } from "../../app/auth";
import { pageItems, queryString, usePagedQuery } from "../../shared/api";
import { DetailLink } from "../../shared/navigation";
import { PagedTable } from "../../shared/paged-table";
import { compactSearchParams } from "../../shared/search-params";
import { TimeWindowFilter, useTimeWindow } from "../../shared/time-window";
import type { LogRecord } from "../../shared/types";
import { Badge, ExpandableText, PageHeader, Panel, formatTime, json } from "../../shared/ui";
import "./logs.css";

const severities = ["TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"];
const filterNames = ["service", "severity", "search", "traceId", "spanId"];

export function LogsPage() {
  const { apiKey } = useAuth();
  const [params, setParams] = useSearchParams();
  const window = useTimeWindow();
  const traceID = params.get("traceId") || "";
  // A trace or span link is already specific, so it opens without a time bound until the operator picks one.
  const anyTime = Boolean(traceID) && (!params.has("range") || params.get("range") === "any");
  const search = params.get("search") || "";
  const longWindow = !anyTime && window.range === "7d";
  const values = Object.fromEntries(filterNames.map((name) => [name, params.get(name) || undefined]));
  const path = `/api/v1/logs?${queryString({ ...values, from: anyTime ? undefined : window.from, to: anyTime ? undefined : window.to })}`;
  const query = usePagedQuery<LogRecord>(["logs"], path, apiKey);
  const logs = pageItems(query.data?.pages);
  const active = filterNames.filter((name) => params.get(name));
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setParams(compactSearchParams(event.currentTarget, params, ["range", "to"]));
  };
  const clear = () => setParams(new URLSearchParams([...params].filter(([key]) => !filterNames.includes(key))));
  return <>
    <PageHeader title="Log explorer">Read logs on their own or follow a log to the trace and span that produced it.</PageHeader>
    <TimeWindowFilter range={anyTime ? "any" : window.range} select={window.select} allowAny={Boolean(traceID)} />
    <form className="toolbar log-filters" key={filterNames.map((name) => params.get(name) || "").join("|")} onSubmit={submit}>
      <input name="service" placeholder="Service" aria-label="Filter logs by service" defaultValue={params.get("service") || ""} />
      <input name="severity" list="log-severities" placeholder="Severity" aria-label="Filter logs by severity" defaultValue={params.get("severity") || ""} />
      <datalist id="log-severities">{severities.map((severity) => <option key={severity} value={severity} />)}</datalist>
      <input name="search" placeholder="Search body text" aria-label="Search log body" defaultValue={search} />
      <input name="traceId" placeholder="Trace ID" aria-label="Filter logs by trace ID" defaultValue={traceID} />
      <input name="spanId" placeholder="Span ID" aria-label="Filter logs by span ID" defaultValue={params.get("spanId") || ""} />
      <button type="submit"><Search size={16} /> Search logs</button>
      {active.length > 0 && <button type="button" className="button-secondary" onClick={clear}><X size={15} /> Clear filters</button>}
    </form>
    {search && (anyTime || longWindow) && <p className="muted log-note" role="note">Body search needs a time range of 24 hours or less. Choose a shorter range if the request is rejected.</p>}
    <Panel title="Logs" meta={`${logs.length} loaded${query.hasNextPage ? " · more available" : ""}`}>
      <PagedTable query={query} empty="No logs match these filters in the selected time range. Widen the range or clear filters."
        headers={["Time", "Service", "Severity", "Body", "Trace"]}
        rows={logs.map((log) => [
          formatTime(log.occurred_at), log.service_name, <Badge value={log.severity_text} />,
          <ExpandableText value={json(log.body)} />,
          log.trace_id ? <DetailLink to={`/traces/${log.trace_id}`}>Open trace</DetailLink> : <span className="muted">Not correlated</span>,
        ])} />
    </Panel>
  </>;
}
