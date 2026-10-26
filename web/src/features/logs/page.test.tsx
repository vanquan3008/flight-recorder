import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { App } from "../../app/app";

const traceID = "0123456789abcdef0123456789abcdef";
const respond = (data: unknown, status = 200) => new Response(JSON.stringify(data), { status });
const long = "x".repeat(900);
const logs = [
  { event_id: "e1", occurred_at: "2026-10-01T00:00:01Z", service_name: "checkout", severity_text: "ERROR", body: { message: "card declined" }, trace_id: traceID },
  { event_id: "e2", occurred_at: "2026-10-01T00:00:00Z", service_name: "worker", severity_text: "INFO", body: long },
];

afterEach(() => { cleanup(); sessionStorage.clear(); window.location.hash = ""; vi.unstubAllGlobals(); });

function open(hash: string, status = 200) {
  sessionStorage.setItem("tracepilot-api-key", "test-key");
  window.location.hash = hash;
  const fetch = vi.fn().mockImplementation((input: string) => new URL(input, "http://localhost").pathname === "/api/v1/logs"
    ? (status === 200 ? respond({ data: logs }) : respond({ error: { message: "log content search range must not exceed 24 hours" } }, status))
    : respond({ data: [] }));
  vi.stubGlobal("fetch", fetch);
  render(<App />);
  return fetch;
}
const logRequest = (fetch: ReturnType<typeof vi.fn>) => new URL(String(fetch.mock.calls.map(([path]) => path).find((path) => String(path).startsWith("/api/v1/logs"))), "http://localhost");

it("reads logs on their own inside the selected time window and links correlated logs to traces", async () => {
  const fetch = open("#/logs?range=1h&to=2026-10-01T01%3A00%3A00Z&service=checkout&severity=ERROR");
  expect(await screen.findByText(/card declined/)).toBeTruthy();
  const url = logRequest(fetch);
  expect(url.searchParams.get("from")).toBe("2026-10-01T00:00:00.000Z");
  expect(url.searchParams.get("to")).toBe("2026-10-01T01:00:00.000Z");
  expect(url.searchParams.get("service")).toBe("checkout");
  expect(url.searchParams.get("severity")).toBe("ERROR");
  expect(screen.getByRole("link", { name: "Open trace" }).getAttribute("href")).toBe(`#/traces/${traceID}`);
  expect(screen.getByText("Not correlated")).toBeTruthy();
});

it("collapses a very long log body until the operator expands it", async () => {
  open("#/logs");
  const expand = await screen.findByRole("button", { name: /Show all \(900 chars\)/ });
  expect(screen.queryByText(long)).toBeNull();
  fireEvent.click(expand);
  expect(await screen.findByText(long)).toBeTruthy();
  expect(screen.getByRole("button", { name: "Show less" })).toBeTruthy();
});

it("opens a trace's logs without a time bound and keeps the span filter", async () => {
  const fetch = open(`#/logs?traceId=${traceID}&spanId=0123456789abcdef`);
  await screen.findByText(/card declined/);
  const url = logRequest(fetch);
  expect(url.searchParams.get("traceId")).toBe(traceID);
  expect(url.searchParams.get("spanId")).toBe("0123456789abcdef");
  expect(url.searchParams.has("from")).toBe(false);
  expect((screen.getByLabelText("Time range") as HTMLSelectElement).value).toBe("any");
});

it("explains a rejected body search and lets the operator clear filters", async () => {
  open("#/logs?range=7d&search=timeout", 400);
  expect(await screen.findByText(/must not exceed 24 hours/)).toBeTruthy();
  expect(screen.getByRole("note").textContent).toContain("24 hours or less");
  fireEvent.click(screen.getByRole("button", { name: /Clear filters/ }));
  await waitFor(() => expect(window.location.hash).not.toContain("search="));
  expect(window.location.hash).toContain("range=7d");
});
