import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { useAuth } from "../../shared/session";
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
  return <TraceWorkspace key={traceID} traceID={traceID} />;
}

function TraceWorkspace({ traceID }: { traceID: string }) {
  const { credential } = useAuth();
  const [focus, setFocus] = useState("");
  const [logFilter, setLogFilter] = useState("");
  const trace = useQuery({ queryKey: ["trace", traceID], queryFn: ({ signal }) => loadTrace(traceID, credential, signal) });
  const logs = usePagedQuery<LogRecord>(["trace-logs", traceID], `/api/v1/traces/${traceID}/logs?limit=50`, credential);
  const model = useMemo(() => buildTrace(trace.data?.spans ?? []), [trace.data]);
  const logRows = pageItems(logs.data?.pages);
  const logFilterProps = { search: logFilter, onSearch: setLogFilter };
  const header = <PageHeader title="Trace detail" action={<BackLink fallback="/traces" fallbackLabel="traces" />}>{traceID}</PageHeader>;
  if (trace.isPending || trace.isError) {
    const missing = trace.error instanceof APIError && trace.error.status === 404;
    return <div className="flex min-w-0 flex-col gap-4 pb-4">
      {header}
      {trace.isPending ? <Panel title="Loading trace spans"><Loading /></Panel>
        : missing ? <Panel title="Trace not available"><div className="not-found p-5 [&_p]:mb-3 [&_p]:text-[13px] [&_p]:text-muted">
          <p>Trace spans may still be arriving, may be older than the retention period, or the ID may be mistyped.</p>
          <div className="flex flex-wrap gap-4 text-xs text-accent">
            <button type="button" className="underline" disabled={trace.isFetching} onClick={() => void trace.refetch()}>{trace.isFetching ? "Checking for spans…" : "Check for spans again"}</button>
            <DetailLink to="/traces">Search traces</DetailLink>
          </div>
        </div></Panel>
        : <ErrorState error={trace.error} retry={() => void trace.refetch()} />}
      <TraceLogs traceID={traceID} logs={logRows} query={logs} {...logFilterProps} />
    </div>;
  }
  return <div className="flex min-w-0 flex-col gap-4 pb-4">
    <TraceHeader model={model} traceID={traceID} />
    {trace.data?.truncated && <p className="m-0 rounded-xl border border-warning bg-warn-bg px-4 py-3 text-xs text-warn-fg" role="status">Loaded {model.rows.length.toLocaleString("en")} of the trace’s spans in start-time order. The waterfall, service flow and counts are incomplete. This view loads up to {maxTraceSpans.toLocaleString("en")} spans.</p>}
    {model.rows.length > 0 && <TraceFlow model={model} focus={focus} onFocus={setFocus} />}
    <Waterfall model={model} traceID={traceID} focus={focus} onFocus={setFocus}>
      <TraceLogs traceID={traceID} logs={logRows} query={logs} {...logFilterProps} embedded />
    </Waterfall>
  </div>;
}
