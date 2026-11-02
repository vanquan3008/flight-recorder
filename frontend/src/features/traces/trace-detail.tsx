import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { useAuth } from "../../app/auth";
import { APIError, pageItems, usePagedQuery } from "../../shared/api";
import type { LogRecord } from "../../shared/types";
import { BackLink, Badge, DetailLink, ErrorState, ExpandableText, Loading, PageHeader, PagedTable, Panel, formatTime, json } from "../../shared/ui";
import { TraceFlow } from "./trace-flow";
import { buildTrace } from "./trace-model";
import { loadTrace, maxTraceSpans } from "./trace-pages";
import { Waterfall } from "./waterfall";
import "./traces.css";

export function TraceDetailPage() {
  const { traceID = "" } = useParams();
  const { credential } = useAuth();
  const [focus, setFocus] = useState("");
  const trace = useQuery({ queryKey: ["trace", traceID], queryFn: ({ signal }) => loadTrace(traceID, credential, signal) });
  const logs = usePagedQuery<LogRecord>(["trace-logs", traceID], `/api/v1/traces/${traceID}/logs?limit=50`, credential);
  const model = useMemo(() => buildTrace(trace.data?.spans ?? []), [trace.data]);
  if (trace.isPending) return <Loading page />;
  const header = <PageHeader title="Trace detail" action={<BackLink fallback="/traces" fallbackLabel="traces" />}>{traceID}</PageHeader>;
  if (trace.isError) {
    const missing = trace.error instanceof APIError && trace.error.status === 404;
    return <>{header}{missing
      ? <Panel title="Trace not available"><div className="not-found"><p>This trace is not stored. It may be older than the retention period, still being ingested, or the ID may be mistyped.</p><DetailLink to="/traces">Search traces</DetailLink></div></Panel>
      : <ErrorState error={trace.error} retry={() => void trace.refetch()} />}</>;
  }
  const logRows = pageItems(logs.data?.pages);
  return <>
    {header}
    {trace.data?.truncated && <p className="trace-truncated" role="status">This trace has more than {maxTraceSpans.toLocaleString("en")} spans. Showing the first {maxTraceSpans.toLocaleString("en")} by start time, so the waterfall, service flow and counts below are incomplete.</p>}
    {model.rows.length > 0 && <TraceFlow model={model} focus={focus} onFocus={setFocus} />}
    <Panel title="Span waterfall" meta={`${model.rows.length} spans${focus ? ` · highlighting ${focus}` : ""}`}><Waterfall model={model} traceID={traceID} focus={focus} onFocus={setFocus} /></Panel>
    <Panel title="Correlated logs" meta={`${logRows.length} loaded`}><PagedTable query={logs} empty="No logs are linked to this trace." headers={["Time", "Service", "Severity", "Body"]} rows={logRows.map((log) => [formatTime(log.occurred_at), log.service_name, <Badge value={log.severity_text} />, <ExpandableText value={json(log.body)} />])} /></Panel>
  </>;
}
