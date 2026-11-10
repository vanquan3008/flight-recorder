import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { App } from "../../app/app";

const traceID1 = "0123456789abcdef0123456789abcdef";
const traceID2 = "fedcba9876543210fedcba9876543210";
const respond = (data: unknown, status = 200) => new Response(JSON.stringify(data), { status });

afterEach(() => {
  cleanup();
  sessionStorage.clear();
  window.history.pushState(null, "", "/");
  window.location.hash = "";
  vi.unstubAllGlobals();
});

function open(hash = "#/traces") {
  localStorage.setItem("tracepilot-signed-in", "1");
  const path = hash.startsWith("#") ? hash.slice(1) : hash;
  window.history.pushState(null, "", path);
  window.location.hash = "";

  const fetchMock = vi.fn().mockImplementation((input: string) => {
    const url = new URL(input, "http://localhost");
    if (url.pathname === "/api/v1/traces") {
      const cursor = url.searchParams.get("cursor");
      if (cursor === "cursor-page-2") {
        return respond({
          data: [
            {
              trace_id: traceID2,
              started_at: "2026-10-01T00:05:00Z",
              ended_at: "2026-10-01T00:05:01Z",
              duration_nanos: 5e8,
              root_service: "worker",
              span_count: 1,
              error_count: 0,
            },
          ],
        });
      }
      return respond({
        data: [
          {
            trace_id: traceID1,
            started_at: "2026-10-01T00:10:00Z",
            ended_at: "2026-10-01T00:10:01Z",
            duration_nanos: 1e9,
            root_service: "checkout",
            span_count: 3,
            error_count: 1,
          },
        ],
        next_cursor: "cursor-page-2",
      });
    }
    return respond({ data: [] });
  });

  vi.stubGlobal("fetch", fetchMock);
  render(<App />);
  return fetchMock;
}

it("renders the trace list with pagination controls and handles Next / Prev navigation", async () => {
  open();

  // Page 1 should display trace 1
  expect(await screen.findByText("checkout")).toBeTruthy();
  expect(screen.getByText(traceID1)).toBeTruthy();
  expect(screen.getByText("Page 1 · 1 traces")).toBeTruthy();

  // Prev button should be disabled on page 1
  const prevBtn = screen.getByRole("button", { name: "Previous page" });
  expect(prevBtn).toHaveProperty("disabled", true);

  // Next button should be enabled
  const nextBtn = screen.getByRole("button", { name: "Next page" });
  expect(nextBtn).toHaveProperty("disabled", false);

  // Click Next -> loads page 2
  fireEvent.click(nextBtn);
  expect(await screen.findByText("worker")).toBeTruthy();
  expect(screen.getByText(traceID2)).toBeTruthy();
  expect(screen.getByText("Page 2 · 1 traces")).toBeTruthy();

  // Now Prev button should be enabled
  expect(prevBtn).toHaveProperty("disabled", false);

  // Click Prev -> returns to page 1
  fireEvent.click(prevBtn);
  expect(await screen.findByText("checkout")).toBeTruthy();
  expect(screen.getByText(traceID1)).toBeTruthy();
  expect(screen.getByText("Page 1 · 1 traces")).toBeTruthy();
});

it("supports selecting page size and re-fetches with new limit", async () => {
  const fetchMock = open();
  expect(await screen.findByText("checkout")).toBeTruthy();

  const select = screen.getByLabelText("Rows per page");
  expect((select as HTMLSelectElement).value).toBe("15");

  fireEvent.change(select, { target: { value: "25" } });
  await waitFor(() => {
    const traceCalls = fetchMock.mock.calls
      .map(([p]) => String(p))
      .filter((p) => p.includes("/api/v1/traces"));
    const lastCall = traceCalls[traceCalls.length - 1];
    expect(lastCall).toContain("limit=25");
  });
});
