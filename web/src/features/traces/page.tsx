import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, Copy, Search } from "lucide-react";
import { type FormEvent, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { useAuth } from "../../app/auth";
import { api, pageItems, queryString, usePagedQuery } from "../../shared/api";
import { compactSearchParams } from "../../shared/search-params";
import type { LogRecord, Span, TraceDetail, TraceSummary } from "../../shared/types";
import { PagedTable } from "../../shared/paged-table";
import { Badge, Empty, ErrorState, Loading, PageHeader, Panel, formatDuration, formatTime, json } from "../../shared/ui";

export function TracesPage() {
  const { apiKey } = useAuth();
  const [params, setParams] = useSearchParams();
  const path = `/api/v1/traces?${queryString({ service: params.get("service") || undefined, status: params.get("status") || undefined, traceId: params.get("traceId") || undefined })}`;
  const query = usePagedQuery<TraceSummary>(["traces"], path, apiKey);
  const traces = pageItems(query.data?.pages);
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setParams(compactSearchParams(event.currentTarget));
  };
  return <>
    <PageHeader title="Trace explorer">Find slow or failed requests, then inspect their spans and correlated logs.</PageHeader>
    <form className="toolbar" onSubmit={submit}>
      <input name="service" placeholder="Service" aria-label="Filter by service" defaultValue={params.get("service") || ""} />
      <input name="status" placeholder="Status" aria-label="Filter by status" defaultValue={params.get("status") || ""} />
      <input name="traceId" placeholder="Trace ID" aria-label="Filter by trace ID" defaultValue={params.get("traceId") || ""} />
      <button><Search size={16} /> Search traces</button>
    </form>
    <Panel title="Recent traces" meta={`${traces.length} loaded`}><PagedTable query={query} empty={<>No traces match those filters. <Link to="/overview">Review the setup steps.</Link></>} headers={["Started", "Service", "Duration", "Spans", "Errors", "Trace ID", ""]} rows={traces.map((trace) => [
      formatTime(trace.started_at), trace.root_service, formatDuration(trace.duration_nanos), trace.span_count, <Badge value={trace.error_count ? "ERROR" : "OK"} />, <code>{trace.trace_id}</code>, <Link className="table-link" to={`/traces/${trace.trace_id}`}>Inspect</Link>,
    ])} /></Panel>
  </>;
}

export function TraceDetailPage() {
  const { traceID = "" } = useParams();
  const { apiKey } = useAuth();
  const trace = useQuery({ queryKey: ["trace", traceID], queryFn: ({ signal }) => api<TraceDetail>(`/api/v1/traces/${traceID}`, apiKey, { signal }) });
  const logs = usePagedQuery<LogRecord>(["trace-logs", traceID], `/api/v1/traces/${traceID}/logs?limit=50`, apiKey);
  if (trace.isPending) return <Loading />;
  if (trace.isError) return <ErrorState error={trace.error} retry={() => void trace.refetch()} />;
  const logRows = pageItems(logs.data?.pages);
  return <>
    <PageHeader title="Trace detail" action={<Link className="back-link" to="/traces"><ArrowLeft size={16} /> Back to traces</Link>}>{traceID}</PageHeader>
    <Panel title="Span waterfall" meta={`${trace.data.spans.length} spans`}><Waterfall spans={trace.data.spans} /></Panel>
    <Panel title="Correlated logs" meta={`${logRows.length} loaded`}><PagedTable query={logs} empty="No logs are linked to this trace." headers={["Time", "Service", "Severity", "Body"]} rows={logRows.map((log) => [formatTime(log.occurred_at), log.service_name, <Badge value={log.severity_text} />, <pre className="inline-json">{json(log.body)}</pre>])} /></Panel>
  </>;
}

function Waterfall({ spans }: { spans: Span[] }) {
  const [selected, setSelected] = useState<Span | null>(null);
  if (!spans.length) return <Empty>This trace has no spans.</Empty>;
  const byParent = new Map<string, Span[]>();
  const ids = new Set(spans.map((span) => span.span_id));
  spans.forEach((span) => {
    const parent = ids.has(span.parent_span_id || "") ? span.parent_span_id || "" : "";
    byParent.set(parent, [...(byParent.get(parent) || []), span]);
  });
  byParent.forEach((children) => children.sort((left, right) => Date.parse(left.started_at) - Date.parse(right.started_at)));
  const ordered: Array<[Span, number]> = [];
  const seen = new Set<string>();
  const visit = (parent: string, depth: number) => (byParent.get(parent) || []).forEach((span) => {
    if (seen.has(span.span_id)) return;
    seen.add(span.span_id);
    ordered.push([span, depth]);
    visit(span.span_id, depth + 1);
  });
  visit("", 0);
  spans.forEach((span) => !seen.has(span.span_id) && ordered.push([span, 0]));
  const startedAt = Math.min(...spans.map((span) => Date.parse(span.started_at)));
  const endedAt = Math.max(...spans.map((span) => Date.parse(span.ended_at)));
  const total = Math.max(endedAt - startedAt, 1);
  return <div className="waterfall">{ordered.map(([span, depth]) => {
    const start = Date.parse(span.started_at);
    const end = Date.parse(span.ended_at);
    return <button className="waterfall-row" type="button" onClick={() => setSelected(span)} key={span.span_id} aria-pressed={selected?.span_id === span.span_id}>
      <span className="waterfall-label" style={{ paddingLeft: `${depth * 14}px` }}>{span.name}</span>
      <span className="waterfall-rail"><span className={`waterfall-bar ${span.status_code === "ERROR" ? "error" : ""}`} style={{ marginLeft: `${((start - startedAt) / total) * 100}%`, width: `${Math.max(((end - start) / total) * 100, 1)}%` }} /></span>
      <span className="waterfall-duration">{formatDuration(Math.max(end - start, 0) * 1e6)}</span>
    </button>;
  })}{selected && <SpanDetail span={selected} />}</div>;
}

function SpanDetail({ span }: { span: Span }) {
  return <section className="span-detail"><div className="detail-heading"><div><h3>{span.name}</h3><p>{span.service_name} · {span.status_code || "UNSET"}</p></div><CopyButton value={span.span_id} /></div>{[["Attributes", span.attributes], ["Resource", span.resource_attributes], ["Events", span.events], ["Links", span.links]].map(([name, value]) => <div key={String(name)}><h4>{String(name)}</h4><pre>{json(value)}</pre></div>)}</section>;
}

function CopyButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);
  return <button className="icon-button" aria-label="Copy span ID" onClick={() => void navigator.clipboard.writeText(value).then(() => { setCopied(true); window.setTimeout(() => setCopied(false), 1200); })}><Copy size={16} />{copied && <span className="sr-only">Copied</span>}</button>;
}
