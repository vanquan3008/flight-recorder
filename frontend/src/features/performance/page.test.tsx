import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { App } from "../../app/app";
import { emptyPerformance } from "../../testing/performance";

afterEach(() => { cleanup(); sessionStorage.clear(); window.location.hash = ""; vi.unstubAllGlobals(); });

it("opens as its own page from the sidebar and keeps working when telemetry quality fails", async () => {
  localStorage.setItem("tracepilot-signed-in", "1");
  window.location.hash = "#/performance?range=1h&to=2026-10-01T01%3A00%3A00Z";
  const fetch = vi.fn().mockImplementation((input: string) => input.startsWith("/api/v1/telemetry/quality")
    ? new Response(JSON.stringify({ error: { message: "unavailable" } }), { status: 503 })
    : new Response(JSON.stringify(input.startsWith("/api/v1/performance") ? emptyPerformance : { data: [] }), { status: 200 }));
  vi.stubGlobal("fetch", fetch);
  render(<App />);
  expect(await screen.findByRole("heading", { level: 1, name: "Performance" })).toBeTruthy();
  expect(await screen.findByText("Observed requests")).toBeTruthy();
  expect(screen.getByRole("link", { name: "Performance" }).getAttribute("aria-current")).toBe("page");
  const request = new URL(String(fetch.mock.calls.find(([path]) => String(path).startsWith("/api/v1/performance"))![0]), "http://localhost");
  expect(request.searchParams.get("from")).toBe("2026-10-01T00:00:00.000Z");
  expect(request.searchParams.get("to")).toBe("2026-10-01T01:00:00.000Z");
  expect(screen.queryByText("Telemetry quality")).toBeNull();
});
