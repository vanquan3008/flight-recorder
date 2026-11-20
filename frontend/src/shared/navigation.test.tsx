import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { App } from "../app/app";

const traceID = "0123456789abcdef0123456789abcdef";
const incidentID = "01900000-0000-7000-8000-000000000001";
const respond = (data: unknown, status = 200) => new Response(JSON.stringify(data), { status });

afterEach(() => { cleanup(); sessionStorage.clear(); window.history.pushState(null, "", "/"); window.location.hash = ""; vi.unstubAllGlobals(); });

function connect(hash: string) {
  localStorage.setItem("tracepilot-signed-in", "1");
  const path = hash.startsWith("#") ? hash.slice(1) : hash;
  window.history.pushState(null, "", path);
  window.location.hash = "";
}

it("returns from a trace to the filtered, time-bounded trace list", async () => {
  connect("#/traces?service=checkout&status=ERROR&from=2026-10-01T00%3A00%3A00.000Z&to=2026-10-01T01%3A00%3A00.000Z");
  vi.stubGlobal("fetch", vi.fn().mockImplementation((input: string) => {
    const path = new URL(input, "http://localhost").pathname;
    if (path === "/api/v1/traces") return respond({ data: [{ trace_id: traceID, started_at: "2026-10-01T00:10:00Z", ended_at: "2026-10-01T00:10:01Z", duration_nanos: 1e9, root_service: "checkout", span_count: 2, error_count: 1 }] });
    if (path === `/api/v1/traces/${traceID}`) return respond({ trace_id: traceID, spans: [] });
    return respond({ data: [] });
  }));
  render(<App />);
  fireEvent.click(await screen.findByRole("link", { name: "Inspect" }));
  expect(await screen.findByRole("heading", { name: "Trace detail" })).toBeTruthy();
  fireEvent.click(screen.getByRole("link", { name: /Back to traces/ }));
  expect(await screen.findByLabelText("Filter by service")).toBeTruthy();
  await waitFor(() => expect(window.location.search).toContain("service=checkout"));
  expect(window.location.search).toContain("status=ERROR");
  expect(window.location.search).toContain("from=2026-10-01T00");
  expect((screen.getByLabelText("Filter by service") as HTMLInputElement).value).toBe("checkout");
});

it("returns from an incident to the filtered incident list and labels a direct visit generically", async () => {
  connect("#/incidents?status=OPEN&service=checkout");
  const record = { id: incidentID, title: "Checkout failed", status: "OPEN", severity: "ERROR", primary_service: "checkout", occurrence_count: 1, first_seen_at: "2026-10-01T00:00:00Z", last_seen_at: "2026-10-01T00:01:00Z" };
  vi.stubGlobal("fetch", vi.fn().mockImplementation((input: string) => {
    const path = new URL(input, "http://localhost").pathname;
    if (path === "/api/v1/incidents") return respond({ data: [record] });
    if (path === `/api/v1/incidents/${incidentID}`) return respond(record);
    if (path.endsWith("/analysis")) return respond({ incident_id: incidentID, conclusion: "", hypotheses: [], evidence: [], causal_graph: [] });
    return respond({ data: [] });
  }));
  render(<App />);
  fireEvent.click(await screen.findByRole("link", { name: "Inspect" }));
  fireEvent.click(await screen.findByRole("link", { name: /Back to incidents/ }));
  expect(await screen.findByLabelText("Filter by service")).toBeTruthy();
  await waitFor(() => expect(window.location.search).toContain("status=OPEN"));
  expect(window.location.search).toContain("service=checkout");
});

it("shows a recovery path when a trace is no longer stored", async () => {
  connect(`#/traces/${traceID}`);
  vi.stubGlobal("fetch", vi.fn().mockImplementation((input: string) => new URL(input, "http://localhost").pathname === `/api/v1/traces/${traceID}`
    ? respond({ error: { message: "trace not found" } }, 404) : respond({ data: [] })));
  render(<App />);
  expect(await screen.findByText(/older than the retention period/)).toBeTruthy();
  expect(screen.getByRole("link", { name: /Back to traces/ })).toBeTruthy();
});
