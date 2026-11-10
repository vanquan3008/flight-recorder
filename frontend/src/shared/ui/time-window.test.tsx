import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { TimeWindowFilter, useTimeWindow } from "./time-window";

function Window() {
  const window = useTimeWindow();
  return <><output>{JSON.stringify({ from: window.from, to: window.to })}</output><TimeWindowFilter {...window} /></>;
}
afterEach(() => { cleanup(); vi.useRealTimers(); });

it("keeps the absolute 24-hour window passed by topology", () => {
  render(<MemoryRouter initialEntries={["/logs?from=2026-10-01T00:00:00Z&to=2026-10-02T00:00:00Z"]}><Window /></MemoryRouter>);
  expect(JSON.parse(screen.getByRole("status").textContent!)).toEqual({ from: "2026-10-01T00:00:00.000Z", to: "2026-10-02T00:00:00.000Z" });
  expect(screen.getByRole("radio", { name: "Last 24 hours" })).toHaveProperty("checked", true);
  fireEvent.click(screen.getByRole("radio", { name: "Last hour" }));
  const selected = JSON.parse(screen.getByRole("status").textContent!);
  expect(Date.parse(selected.to) - Date.parse(selected.from)).toBe(3600000);
});

it("refreshes a custom interval without shortening it to the default preset", () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-05T00:00:00Z"));
  render(<MemoryRouter initialEntries={["/logs?from=2026-10-01T00:00:00Z&to=2026-10-01T02:30:00Z"]}><Window /></MemoryRouter>);
  expect(screen.getByLabelText("Custom time range")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Refresh window" }));
  expect(JSON.parse(screen.getByRole("status").textContent!)).toEqual({ from: "2026-10-04T21:30:00.000Z", to: "2026-10-05T00:00:00.000Z" });
});
