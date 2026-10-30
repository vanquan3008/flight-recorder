import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { App } from "./app";
import { emptyPerformance } from "../features/performance/test-data";

afterEach(() => {
  cleanup();
  sessionStorage.clear();
  window.location.hash = "";
  vi.unstubAllGlobals();
});

describe("App", () => {
  it("keeps protected routes behind the session-only API key prompt", () => {
    render(<App />);
    expect(screen.getByRole("heading", { name: "Welcome to TracePilot" })).toBeTruthy();
    expect(screen.getByLabelText("API key")).toBeTruthy();
  });

  it("guides a connected operator from an empty console to the first trace", async () => {
    sessionStorage.setItem("tracepilot-api-key", "test-key");
    window.location.hash = "#/overview";
    vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string) => new Response(JSON.stringify(path.startsWith("/api/v1/performance") ? emptyPerformance : { data: [] }), { status: 200 })));
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Get your first trace" })).toBeTruthy();
    expect(screen.getByText(/Configure OTLP/)).toBeTruthy();
    expect(screen.getByText("Send a request from your instrumented application, then refresh this page.")).toBeTruthy();
  });

  it("shows the product flow without requiring an API key", async () => {
    window.location.hash = "#/flow";
    render(<App />);

    expect(await screen.findByRole("heading", { name: /From signal to answer/ })).toBeTruthy();
    expect(screen.getByText("OTLP ingestion API")).toBeTruthy();
    expect(screen.getByText("Management REST API")).toBeTruthy();
    expect(screen.getByText("Incident and RCA")).toBeTruthy();
  });
});
