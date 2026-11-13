import { expect, it } from "vitest";
import type { LogRecord } from "../../shared/types";
import { matchesLogFilters, mergeLogs } from "./log-model";

const log: LogRecord = { event_id: "e1", occurred_at: "2026-10-01T00:00:00Z", service_name: "checkout", severity_text: "ERROR", trace_id: "trace", span_id: "span", body: { message: "Payment failed" }, resource_attributes: {} };

it("applies every explorer filter to live events", () => {
  const params = new URLSearchParams({ service: "checkout", severity: "ERROR", traceId: "trace", spanId: "span", search: "payment" });
  expect(matchesLogFilters(log, params)).toBe(true);
  for (const field of ["service", "severity", "traceId", "spanId", "search"]) {
    const changed = new URLSearchParams(params);
    changed.set(field, "does-not-match");
    expect(matchesLogFilters(log, changed), field).toBe(false);
  }
});

it("shows a persisted event once when it is also received live", () => {
  const other = { ...log, event_id: "e2" };
  expect(mergeLogs([log], [log, other]).map((item) => item.event_id)).toEqual(["e1", "e2"]);
});
