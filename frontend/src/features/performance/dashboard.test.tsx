import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { AuthContext } from "../../app/auth";
import { PerformanceDashboard } from "./dashboard";
import { emptyPerformance } from "./test-data";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

function renderDashboard() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><AuthContext.Provider value={{ credential: "session", isAuthenticated: true }}>
    <MemoryRouter><PerformanceDashboard from={emptyPerformance.from} to={emptyPerformance.to} /></MemoryRouter>
  </AuthContext.Provider></QueryClientProvider>);
}

it("shows request-only rates/percentiles, charts, and exact service/endpoint filters", async () => {
  const stats = { requests: 120, errors: 6, requests_per_second: 0.0333333, error_rate: 0.05, average_latency_nanos: 100e6, p95_latency_nanos: 250e6, p99_latency_nanos: 400e6 };
  const response = {
    ...emptyPerformance, summary: stats,
    services: [{ ...stats, service: "checkout" }], endpoints: [{ ...stats, service: "checkout", endpoint: "GET /orders" }],
    series: [{ ...stats, from: emptyPerformance.from, to: emptyPerformance.to }],
  };
  const fetch = vi.fn().mockImplementation(() => new Response(JSON.stringify(response), { status: 200 }));
  vi.stubGlobal("fetch", fetch);
  renderDashboard();
  expect(await screen.findByText("Observed requests")).toBeTruthy();
  const card = (name: string) => within(screen.getByText(name).closest(".stat-card") as HTMLElement);
  expect(card("Observed requests").getByText("120")).toBeTruthy();
  expect(card("Request rate").getByText("0.0333")).toBeTruthy();
  expect(card("Request error rate").getByText("5.00%")).toBeTruthy();
  expect(card("P95 latency").getByText("250.0ms")).toBeTruthy();
  expect(card("P99 latency").getByText("400.0ms")).toBeTruthy();
  expect(screen.getByRole("img", { name: "Observed request rate over time" })).toBeTruthy();
  expect(screen.getByRole("img", { name: "Observed request latency over time" })).toBeTruthy();
  expect(screen.getByText("GET /orders")).toBeTruthy();
  expect(screen.getByText(/Sampling or missing telemetry/)).toBeTruthy();
  fireEvent.change(screen.getByLabelText("Performance service"), { target: { value: "checkout" } });
  fireEvent.change(screen.getByLabelText("Performance endpoint"), { target: { value: "GET /orders" } });
  fireEvent.click(screen.getByRole("button", { name: "Apply performance filters" }));
  await waitFor(() => expect(fetch.mock.calls.length).toBe(2));
  const url = new URL(fetch.mock.calls[1][0], "http://localhost");
  expect(url.searchParams.get("service")).toBe("checkout");
  expect(url.searchParams.get("endpoint")).toBe("GET /orders");
  expect(url.searchParams.get("from")).toBe(emptyPerformance.from);
  expect(url.searchParams.get("to")).toBe(emptyPerformance.to);
  fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));
  await waitFor(() => expect((screen.getByLabelText("Performance service") as HTMLInputElement).value).toBe(""));
});

it("keeps empty windows distinct from zero-latency requests", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify(emptyPerformance), { status: 200 })));
  renderDashboard();
  await screen.findByText("Observed requests");
  expect(screen.queryByRole("img")).toBeNull();
  expect(screen.getByText("P95 latency").closest(".stat-card")?.textContent).toContain("—");
  expect(screen.getByText(/No inbound requests in this window/)).toBeTruthy();
  expect(document.body.textContent).not.toMatch(/NaN|Infinity/);
});

it("shows query failures without reporting zero traffic and supports retry", async () => {
  const fetch = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ error: { message: "Database unavailable" } }), { status: 503 }))
    .mockResolvedValueOnce(new Response(JSON.stringify(emptyPerformance), { status: 200 }));
  vi.stubGlobal("fetch", fetch);
  renderDashboard();
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(screen.queryByText("Observed requests")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  expect(await screen.findByText("Observed requests")).toBeTruthy();
  expect(screen.queryByRole("alert")).toBeNull();
});

it("shows a layout skeleton first, then keeps the previous figures dimmed while a new filter loads", async () => {
  let release: (response: Response) => void = () => {};
  const fetch = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify(emptyPerformance), { status: 200 }))
    .mockReturnValueOnce(new Promise<Response>((resolve) => { release = resolve; }));
  vi.stubGlobal("fetch", fetch);
  renderDashboard();
  expect(screen.getByRole("status").textContent).toContain("Loading performance data");
  await screen.findByText("Observed requests");
  expect(screen.queryByRole("status")).toBeNull();
  fireEvent.change(screen.getByLabelText("Performance service"), { target: { value: "checkout" } });
  fireEvent.click(screen.getByRole("button", { name: "Apply performance filters" }));
  await waitFor(() => expect(document.querySelector(".performance-content.is-stale")).not.toBeNull());
  expect(screen.getByText("Observed requests")).toBeTruthy();
  release(new Response(JSON.stringify(emptyPerformance), { status: 200 }));
  await waitFor(() => expect(document.querySelector(".performance-content.is-stale")).toBeNull());
});
