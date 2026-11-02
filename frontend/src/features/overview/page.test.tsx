import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { App } from "../../app/app";
import { emptyPerformance } from "../performance/test-data";

afterEach(() => { cleanup(); sessionStorage.clear(); window.location.hash = ""; vi.unstubAllGlobals(); });

it("uses one selected window and labels the complete metric count accurately", async () => {
  localStorage.setItem("tracepilot-signed-in", "1");
  window.location.hash = "#/overview?range=1h&to=2026-10-01T01%3A00%3A00Z";
  const fetch = vi.fn().mockImplementation((input: string) => new Response(JSON.stringify(
    input.startsWith("/api/v1/telemetry/quality") ? { metric_points: 123 } : input.startsWith("/api/v1/performance") ? emptyPerformance : { data: [] },
  ), { status: 200 }));
  vi.stubGlobal("fetch", fetch);
  render(<App />);
  expect(await screen.findByText("123")).toBeTruthy();
  expect(screen.getByText("all stored points in window")).toBeTruthy();
  const card = screen.getByText("Metric points").closest(".stat-card")!;
  expect(card.tagName).toBe("DIV");
  await waitFor(() => expect(fetch.mock.calls.some(([path]) => String(path).startsWith("/api/v1/performance"))).toBe(true));
  const endpoints = fetch.mock.calls.map(([path]) => new URL(String(path), "http://localhost")).filter((url) =>
    ["/api/v1/incidents", "/api/v1/traces", "/api/v1/telemetry/quality", "/api/v1/performance"].includes(url.pathname) && url.searchParams.has("from"));
  expect(endpoints.length).toBe(4);
  for (const url of endpoints) {
    expect(url.searchParams.get("from")).toBe("2026-10-01T00:00:00.000Z");
    expect(url.searchParams.get("to")).toBe("2026-10-01T01:00:00.000Z");
  }
  fireEvent.click(screen.getByLabelText("Last 24 hours"));
  await waitFor(() => expect(window.location.hash).toContain("range=24h"));
});
