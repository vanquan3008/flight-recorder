import { api } from "../../shared/api";
import type { Span, TraceDetail } from "../../shared/types";

/** The server's page size limit: the most spans one request returns. */
const pageSize = 500;
/** The most spans the console loads for one trace. */
export const maxTraceSpans = 5000;
/** A page can be shorter than pageSize when its spans are large, so bound the requests too. */
const maxPages = 60;

export type LoadedTrace = { spans: Span[]; truncated: boolean };

/**
 * Reads a trace page by page. `truncated` is true only when the server reported
 * more spans (next_cursor) than the console is willing to hold, so a partial
 * trace is never shown as if it were complete.
 */
export async function loadTrace(traceID: string, credential: string, signal?: AbortSignal): Promise<LoadedTrace> {
  const spans: Span[] = [];
  let cursor = "";
  for (let page = 0; page < maxPages; page++) {
    const query = new URLSearchParams({ limit: String(pageSize) });
    if (cursor) query.set("cursor", cursor);
    const result = await api<TraceDetail>(`/api/v1/traces/${traceID}?${query}`, credential, { signal });
    spans.push(...result.spans);
    if (!result.next_cursor) return { spans, truncated: false };
    if (spans.length >= maxTraceSpans) return { spans, truncated: true };
    cursor = result.next_cursor;
  }
  return { spans, truncated: true };
}
