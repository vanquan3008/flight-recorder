import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { App } from "../../app/app";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { AuthContext } from "../../app/auth";
import { IncidentPage } from "./incident-page";

const incidentID = "01900000-0000-7000-8000-000000000001";
const record = { id: incidentID, title: "Checkout failed", status: "OPEN", severity: "ERROR", primary_service: "checkout", occurrence_count: 2, first_seen_at: "2026-10-01T00:00:00Z", last_seen_at: "2026-10-01T00:01:00Z" };
const emptyAnalysis = { incident_id: incidentID, conclusion: "Insufficient evidence", hypotheses: [], evidence: [], causal_graph: [] };

function connect() {
  localStorage.setItem("tracepilot-signed-in", "1");
  window.location.hash = `#/incidents/${incidentID}`;
}
function response(data: unknown, status = 200) { return new Response(JSON.stringify(data), { status }); }

afterEach(() => { cleanup(); localStorage.clear(); sessionStorage.clear(); window.history.replaceState(null, "", "/"); vi.unstubAllGlobals(); });

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
  fireEvent.click(await screen.findByRole("tab", { name: "Status history" }));
  expect(await screen.findByRole("heading", { name: "Status history" })).toBeTruthy();
  expect(await screen.findByRole("button", { name: "Resolve incident" })).toBeTruthy();
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
    if (url.pathname.endsWith("/status-changes")) return response({ data: resolved ? [{ previous_status: "OPEN", status: "RESOLVED", actor_user_id: "0190aaaa-0000-7000-8000-000000000002", changed_at: "2026-10-01T00:02:00Z" }] : [] });
    if (input === `/api/v1/incidents/${incidentID}`) return response({ ...record, status: resolved ? "RESOLVED" : "OPEN" });
    return response({ data: [] });
  });
  vi.stubGlobal("fetch", fetch);
  render(<App />);
  fireEvent.click(await screen.findByRole("tab", { name: /Occurrences/ }));
  expect(await screen.findByText("first-page-operation")).toBeTruthy();
  const timeline = screen.getByRole("heading", { name: "Occurrence timeline" }).closest("section")!;
  fireEvent.click(within(timeline).getByRole("button", { name: "Next page" }));
  expect(await screen.findByText("second-page-operation")).toBeTruthy();
  expect(screen.queryByText("first-page-operation")).toBeNull();
  fireEvent.click(within(timeline).getByRole("button", { name: "Previous page" }));
  expect(await screen.findByText("first-page-operation")).toBeTruthy();
  fireEvent.click(screen.getByRole("tab", { name: "Status history" }));
  await screen.findByText("No status changes have been recorded.");
  fireEvent.click(screen.getByRole("button", { name: "Resolve incident" }));
  expect(await screen.findByText("user 0190aaaa")).toBeTruthy();
  await waitFor(() => expect(screen.getByRole("button", { name: "Reopen incident" })).toBeTruthy());
  expect(fetch.mock.calls.filter(([path]) => String(path).includes("/status-changes")).length).toBeGreaterThanOrEqual(2);
});

it("only loads the active view and supports keyboard tab navigation", async () => {
  connect();
  const fetch = vi.fn((input: string) => {
    if (input.endsWith("/analysis")) return response(emptyAnalysis);
    if (input === `/api/v1/incidents/${incidentID}`) return response(record);
    return response({ data: [] });
  });
  vi.stubGlobal("fetch", fetch);
  render(<App />);
  await screen.findByRole("heading", { name: "RCA hypotheses" });
  expect(fetch.mock.calls.some(([path]) => /\/(occurrences|events|status-changes)\?/.test(path))).toBe(false);
  const tab = screen.getByRole("tab", { name: "Investigation" });
  fireEvent.keyDown(tab, { key: "End" });
  await screen.findByText("No status changes have been recorded.");
  expect(screen.getByRole("tab", { name: "Status history" }).getAttribute("aria-selected")).toBe("true");
  expect(fetch.mock.calls.some(([path]) => path.includes("/status-changes?"))).toBe(true);
  expect(fetch.mock.calls.some(([path]) => /\/(occurrences|events)\?/.test(path))).toBe(false);
});

it("bounds evidence and hypotheses, follows evidence references and preserves small values", async () => {
  connect();
  const evidence = Array.from({ length: 17 }, (_, index) => ({ id: `evidence-${index}`, signal_type: "metric", entity_type: "service", entity_id: "service:checkout", metric_name: `dns.duration.${index}`, direction: "increase", observed_value: 0.001062, baseline_value: 0.0004316, change_ratio: 2.4606, trace_ids: [], summary: "DNS lookup duration increased" }));
  const hypotheses = Array.from({ length: 5 }, (_, index) => ({ rank: index + 1, entity_id: "service:checkout", statement: `Hypothesis ${index}`, confidence: 0.95, evidence_ids: ["evidence-16"] }));
  vi.stubGlobal("fetch", vi.fn((input: string) => {
    if (input.endsWith("/analysis")) return response({ ...emptyAnalysis, hypotheses, evidence });
    if (input === `/api/v1/incidents/${incidentID}`) return response(record);
    return response({ data: [] });
  }));
  render(<App />);
  await screen.findByText("dns.duration.0");
  expect(document.querySelectorAll(".evidence-card")).toHaveLength(8);
  expect(screen.queryByText("Hypothesis 3")).toBeNull();
  expect(screen.getAllByText("0.0004316")).toHaveLength(8);
  expect(screen.getByText(/Scores are not probabilities/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Next signals" }));
  expect(screen.queryByText("dns.duration.0")).toBeNull();
  expect(screen.getByText("dns.duration.8")).toBeTruthy();
  fireEvent.click(screen.getAllByRole("button", { name: /View supporting evidence/ })[0]);
  expect(document.querySelectorAll(".evidence-card")).toHaveLength(1);
  expect(screen.getByText("dns.duration.16")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Clear selection" }));
  expect(document.querySelectorAll(".evidence-card")).toHaveLength(8);
});

it("keeps the previous incident status and allows other views after a rejected status change", async () => {
  connect();
  vi.stubGlobal("fetch", vi.fn((input: string, init?: RequestInit) => {
    if (init?.method === "PATCH") return response({ error: { message: "Status change denied" } }, 403);
    if (input.endsWith("/analysis")) return response(emptyAnalysis);
    if (input === `/api/v1/incidents/${incidentID}`) return response(record);
    return response({ data: [] });
  }));
  render(<App />);
  fireEvent.click(await screen.findByRole("button", { name: "Resolve incident" }));
  expect(await screen.findByRole("alert")).toHaveProperty("textContent", "Status change denied");
  expect(screen.getByRole("button", { name: "Resolve incident" }).hasAttribute("disabled")).toBe(false);
  expect(screen.queryByRole("button", { name: "Reopen incident" })).toBeNull();
  fireEvent.click(screen.getByRole("tab", { name: "Linked telemetry" }));
  expect(await screen.findByText("No telemetry events are linked to this incident.")).toBeTruthy();
});

it("keeps the drill-down origin and filters when changing investigation tabs", async () => {
  vi.stubGlobal("fetch", vi.fn((input: string) => response(input.endsWith("/analysis") ? emptyAnalysis : input === `/api/v1/incidents/${incidentID}` ? record : { data: [] })));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><AuthContext.Provider value={{ credential: "session", isAuthenticated: true }}>
    <MemoryRouter initialEntries={[{ pathname: `/incidents/${incidentID}`, search: "?project=alpha&range=24h", state: { from: "/overview?project=alpha&range=24h" } }]}>
      <Routes><Route path="/incidents/:incidentID" element={<IncidentPage />} /></Routes>
    </MemoryRouter>
  </AuthContext.Provider></QueryClientProvider>);
  fireEvent.click(await screen.findByRole("tab", { name: "Status history" }));
  await screen.findByText("No status changes have been recorded.");
  expect(screen.getByRole("link", { name: "Back to overview" }).getAttribute("href")).toBe("/overview?project=alpha&range=24h");
  client.clear();
});
