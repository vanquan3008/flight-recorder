import { Search } from "lucide-react";
import type { FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useAuth } from "../../app/auth";
import { compactSearchParams, pageItems, queryString, usePagedQuery } from "../../shared/api";
import type { TraceSummary } from "../../shared/types";
import { Badge, DetailLink, ErrorState, PageHeader, PagedTable, Panel, formatDuration, formatTime } from "../../shared/ui";

export function TracesPage() {
  const { credential } = useAuth();
  const [params, setParams] = useSearchParams();
  const path = `/api/v1/traces?${queryString({ service: params.get("service") || undefined, status: params.get("status") || undefined, traceId: params.get("traceId") || undefined, from: params.get("from") || undefined, to: params.get("to") || undefined })}`;
  const query = usePagedQuery<TraceSummary>(["traces"], path, credential);
  const traces = pageItems(query.data?.pages);
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setParams(compactSearchParams(event.currentTarget, params, ["from", "to"]));
  };
  if (query.isError) return <ErrorState error={query.error} retry={() => void query.refetch()} fullPage />;
  return <>
    <PageHeader title="Trace explorer">Find slow or failed requests, then inspect their spans and correlated logs.</PageHeader>
    <form className="toolbar" onSubmit={submit}>
      <input name="service" placeholder="Service" aria-label="Filter by service" defaultValue={params.get("service") || ""} />
      <input name="status" placeholder="Status" aria-label="Filter by status" defaultValue={params.get("status") || ""} />
      <input name="traceId" placeholder="Trace ID" aria-label="Filter by trace ID" defaultValue={params.get("traceId") || ""} />
      <button><Search size={16} /> Search traces</button>
    </form>
    <Panel title="Recent traces" meta={query.isPending ? undefined : `${traces.length} loaded`}><PagedTable query={query} empty={<>No traces match those filters. <Link to="/overview">Review the setup steps.</Link></>} headers={["Started", "Service", "Duration", "Spans", "Errors", "Trace ID", ""]} rows={traces.map((trace) => [
      formatTime(trace.started_at), trace.root_service, formatDuration(trace.duration_nanos), trace.span_count, <Badge value={trace.error_count ? "ERROR" : "OK"} />, <code>{trace.trace_id}</code>, <DetailLink to={`/traces/${trace.trace_id}`}>Inspect</DetailLink>,
    ])} /></Panel>
  </>;
}

export { TraceDetailPage } from "./trace-detail";
