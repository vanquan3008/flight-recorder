import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { App } from "../../app/app";

const incidentID = "01900000-0000-7000-8000-000000000001";
const record = { id: incidentID, title: "Checkout failed", status: "OPEN", severity: "ERROR", primary_service: "checkout", occurrence_count: 2, first_seen_at: "2026-10-01T00:00:00Z", last_seen_at: "2026-10-01T00:01:00Z" };
const emptyAnalysis = { incident_id: incidentID, conclusion: "Insufficient evidence", hypotheses: [], evidence: [], causal_graph: [] };

function connect() {
  sessionStorage.setItem("tracepilot-api-key", "test-key");
  window.location.hash = `#/incidents/${incidentID}`;
}
function response(data: unknown, status = 200) { return new Response(JSON.stringify(data), { status }); }

afterEach(() => { cleanup(); sessionStorage.clear(); window.location.hash = ""; vi.unstubAllGlobals(); });

it("keeps the incident and timelines usable when analysis fails", async () => {
  connect();
  vi.stubGlobal("fetch", vi.fn().mockImplementation((input: string) => {
    if (input.endsWith("/analysis")) return response({ error: { message: "Analysis unavailable" } }, 501);
    if (input === `/api/v1/incidents/${incidentID}`) return response(record);
    return response({ data: [] });
  }));
  render(<App />);
  expect(await screen.findByRole("heading", { name: "Checkout failed" })).toBeTruthy();
  expect(await screen.findByText("Analysis unavailable")).toBeTruthy();
  expect(screen.getByRole("heading", { name: "Status history" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "RESOLVED" })).toBeTruthy();
});

it("loads the next occurrence page and refreshes status history after a change", async () => {
  connect();
  let resolved = false;
  const fetch = vi.fn().mockImplementation((input: string, init?: RequestInit) => {
    const url = new URL(input, "http://localhost");
    if (init?.method === "PATCH") { resolved = true; return response({ ...record, status: "RESOLVED" }); }
    if (url.pathname.endsWith("/analysis")) return response(emptyAnalysis);
    if (url.pathname.endsWith("/occurrences")) return response({
      data: [{ occurred_at: "2026-10-01T00:00:00Z", service_name: "checkout", operation: url.searchParams.get("cursor") ? "second-page-operation" : "first-page-operation" }],
      next_cursor: url.searchParams.get("cursor") ? undefined : "page-two",
    });
    if (url.pathname.endsWith("/status-changes")) return response({ data: resolved ? [{ previous_status: "OPEN", status: "RESOLVED", actor_key_id: "operator", changed_at: "2026-10-01T00:02:00Z" }] : [] });
    if (input === `/api/v1/incidents/${incidentID}`) return response({ ...record, status: resolved ? "RESOLVED" : "OPEN" });
    return response({ data: [] });
  });
  vi.stubGlobal("fetch", fetch);
  render(<App />);
  expect(await screen.findByText("first-page-operation")).toBeTruthy();
  const timeline = screen.getByRole("heading", { name: "Occurrence timeline" }).closest("section")!;
  fireEvent.click(within(timeline).getByRole("button", { name: "Load more" }));
  expect(await screen.findByText("second-page-operation")).toBeTruthy();
  expect(screen.getByText("first-page-operation")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "RESOLVED" }));
  expect(await screen.findByText("operator")).toBeTruthy();
  await waitFor(() => expect(screen.getByRole("button", { name: "OPEN" })).toBeTruthy());
  expect(fetch.mock.calls.filter(([path]) => String(path).includes("/status-changes")).length).toBeGreaterThanOrEqual(2);
});
