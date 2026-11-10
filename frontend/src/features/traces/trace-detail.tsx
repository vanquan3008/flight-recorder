import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { useAuth } from "../../app/auth";
import { APIError, pageItems, usePagedQuery } from "../../shared/api";
import type { LogRecord } from "../../shared/types";
import { BackLink, DetailLink, ErrorState, Loading, PageHeader, Panel } from "../../shared/ui";
import { TraceFlow } from "./trace-flow";
import { buildTrace } from "./trace-model";
import { loadTrace, maxTraceSpans } from "./trace-pages";
import { Waterfall } from "./waterfall";
import { TraceHeader } from "./trace-header";
import { TraceLogs } from "./trace-logs";

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
      ? <Panel title="Trace not available"><div className="not-found p-5 [&_p]:mb-3 [&_p]:text-[13px] [&_p]:text-muted [&_a]:text-accent"><p>This trace is not stored. It may be older than the retention period, still being ingested, or the ID may be mistyped.</p><DetailLink to="/traces">Search traces</DetailLink></div></Panel>
      : <ErrorState error={trace.error} retry={() => void trace.refetch()} />}</>;
  }
  const logRows = pageItems(logs.data?.pages);
  return <div className="flex min-w-0 flex-col gap-4 pb-4">
    <TraceHeader model={model} traceID={traceID} />
    {trace.data?.truncated && <p className="m-0 rounded-xl border border-warning bg-warn-bg px-4 py-3 text-xs text-warn-fg" role="status">This trace has more than {maxTraceSpans.toLocaleString("en")} spans. Showing the first {maxTraceSpans.toLocaleString("en")} by start time, so the waterfall, service flow and counts below are incomplete.</p>}
    {model.rows.length > 0 && <TraceFlow model={model} focus={focus} onFocus={setFocus} />}
    <Waterfall key={traceID} model={model} traceID={traceID} focus={focus} onFocus={setFocus}>
      <TraceLogs traceID={traceID} logs={logRows} query={logs} embedded />
    </Waterfall>
  </div>;
}
