import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { App } from "../../app/app";

const incident1 = {
  id: "01900000-0000-7000-8000-000000000001",
  title: "Database connection timeout",
  status: "OPEN",
  severity: "ERROR",
  primary_service: "billing",
  occurrence_count: 12,
  first_seen_at: "2026-10-01T00:00:00Z",
  last_seen_at: "2026-10-01T00:05:00Z",
  error_message: "context deadline exceeded",
};

const incident2 = {
  id: "01900000-0000-7000-8000-000000000002",
  title: "Out of memory",
  status: "INVESTIGATING",
  severity: "CRITICAL",
  primary_service: "worker",
  occurrence_count: 55,
  first_seen_at: "2026-10-01T00:02:00Z",
  last_seen_at: "2026-10-01T00:06:00Z",
};

afterEach(() => {
  cleanup();
  localStorage.clear();
  sessionStorage.clear();
  window.history.replaceState(null, "", "/");
  vi.unstubAllGlobals();
});

it("renders incidents list, status metrics, and handles filtering", async () => {
  localStorage.setItem("tracepilot-signed-in", "1");
  window.history.pushState(null, "", "/incidents?range=1h");
  window.location.hash = "";

  const fetchMock = vi.fn().mockImplementation((input: string) => {
    const url = new URL(input, "http://localhost");
    if (url.pathname === "/api/v1/incidents") {
      const status = url.searchParams.get("status");
      if (status === "OPEN") {
        return new Response(JSON.stringify({ data: [incident1] }));
      }
      return new Response(JSON.stringify({ data: [incident1, incident2] }));
    }
    return new Response(JSON.stringify({ data: [] }));
  });

  vi.stubGlobal("fetch", fetchMock);
  render(<App />);

  // Should render page header and incident items
  expect(await screen.findByRole("heading", { name: "Incidents" })).toBeTruthy();
  expect(await screen.findByText("Database connection timeout")).toBeTruthy();
  expect(screen.getByText("Out of memory")).toBeTruthy();

  // Metric counts
  expect(screen.getByText("billing")).toBeTruthy();
  expect(screen.getByText("worker")).toBeTruthy();

  // Filter by OPEN status tab
  const openTab = screen.getByRole("tab", { name: /Open/i });
  fireEvent.click(openTab);

  await waitFor(() => {
    expect(window.location.search).toContain("status=OPEN");
  });
});

it("shows smart empty state with clear filters button", async () => {
  localStorage.setItem("tracepilot-signed-in", "1");
  window.history.pushState(null, "", "/incidents?service=nonexistent");
  window.location.hash = "";

  const fetchMock = vi.fn().mockImplementation(() => new Response(JSON.stringify({ data: [] })));
  vi.stubGlobal("fetch", fetchMock);
  render(<App />);

  expect(await screen.findByText("No incidents match the active filters.")).toBeTruthy();
  const clearButtons = screen.getAllByRole("button", { name: /Clear/i });
  expect(clearButtons.length).toBeGreaterThan(0);

  fireEvent.click(clearButtons[0]);
  await waitFor(() => {
    expect(window.location.search).not.toContain("service=nonexistent");
  });
});

