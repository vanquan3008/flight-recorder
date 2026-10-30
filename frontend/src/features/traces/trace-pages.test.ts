import { afterEach, expect, it, vi } from "vitest";
import { loadTrace, maxTraceSpans } from "./trace-pages";

const traceID = "0123456789abcdef0123456789abcdef";
const spans = (from: number, count: number) => Array.from({ length: count }, (_, index) => ({ span_id: `s${from + index}` }));

/** Serves `total` spans in pages of the requested size, as the API does. */
function serveTrace(total: number) {
  const requests: URL[] = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation((input: string) => {
    const url = new URL(input, "http://localhost");
    requests.push(url);
    const limit = Number(url.searchParams.get("limit"));
    const from = Number(url.searchParams.get("cursor") || 0);
    const body: Record<string, unknown> = { trace_id: traceID, spans: spans(from, Math.min(limit, total - from)) };
    if (from + limit < total) body.next_cursor = String(from + limit);
    return Promise.resolve(new Response(JSON.stringify(body), { status: 200 }));
  }));
  return requests;
}

afterEach(() => vi.unstubAllGlobals());

it("joins every page in order and reports a complete trace", async () => {
  const requests = serveTrace(1203);
  const trace = await loadTrace(traceID, "key");
  expect(trace.truncated).toBe(false);
  expect(trace.spans).toHaveLength(1203);
  expect(trace.spans.map((span) => (span as { span_id: string }).span_id)).toEqual(Array.from({ length: 1203 }, (_, index) => `s${index}`));
  expect(requests.map((url) => url.searchParams.get("cursor"))).toEqual([null, "500", "1000"]);
  expect(requests.every((url) => url.searchParams.get("limit") === "500")).toBe(true);
});

it("stops at the span ceiling and says the trace is cut", async () => {
  const requests = serveTrace(maxTraceSpans + 2500);
  const trace = await loadTrace(traceID, "key");
  expect(trace.truncated).toBe(true);
  expect(trace.spans).toHaveLength(maxTraceSpans);
  expect(requests).toHaveLength(maxTraceSpans / 500);
});

it("does not call a trace of exactly the ceiling truncated", async () => {
  serveTrace(maxTraceSpans);
  expect((await loadTrace(traceID, "key")).truncated).toBe(false);
});

it("never loops forever on a server that keeps offering more", async () => {
  const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify({ trace_id: traceID, spans: spans(0, 1), next_cursor: "again" }), { status: 200 })));
  vi.stubGlobal("fetch", fetchMock);
  const trace = await loadTrace(traceID, "key");
  expect(trace.truncated).toBe(true);
  expect(fetchMock.mock.calls.length).toBeLessThanOrEqual(60);
});
