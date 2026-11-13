import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { TraceLogs } from "./trace-logs";

afterEach(cleanup);
const logs = [
  { event_id: "one", occurred_at: "2026-10-01T00:00:00.123Z", service_name: "checkout", severity_text: "INFO", body: "request ".repeat(100), resource_attributes: {} },
  { event_id: "two", occurred_at: "2026-10-01T00:00:00.124Z", service_name: "payment", severity_text: "ERROR", body: "Payment declined", resource_attributes: {} },
];
const query = { isPending: false, isError: false, error: null, refetch: vi.fn(), hasNextPage: false, isFetchingNextPage: false, fetchNextPage: vi.fn() };

it("keeps long log messages collapsed until the operator opens the row", () => {
  const { container } = render(<MemoryRouter><TraceLogs traceID="trace" logs={logs} query={query} /></MemoryRouter>);
  const detail = container.querySelector("details")!;
  expect(detail.open).toBe(false);
  fireEvent.click(detail.querySelector("summary")!);
  expect(detail.open).toBe(true);
  expect(detail.querySelector("pre")?.textContent).toBe(logs[0].body);
  expect(within(detail).getByRole("button", { name: "Copy log body" })).toBeTruthy();
});

it("filters loaded logs without implying a search across all stored logs", () => {
  const { container } = render(<MemoryRouter><TraceLogs traceID="trace" logs={logs} query={query} /></MemoryRouter>);
  fireEvent.change(screen.getByRole("textbox", { name: "Filter loaded logs" }), { target: { value: "PAYMENT" } });
  expect(container.querySelectorAll("details")).toHaveLength(1);
  expect(screen.getByText(/1 of 2 loaded logs match/)).toBeTruthy();
  fireEvent.change(screen.getByRole("textbox", { name: "Filter loaded logs" }), { target: { value: "nothing matches" } });
  expect(screen.getByText("No loaded logs match this filter.")).toBeTruthy();
});
