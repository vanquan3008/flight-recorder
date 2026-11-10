import { afterEach, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { SpanDetail } from "./span-detail";

afterEach(cleanup);
const span = {
  trace_id: "trace", span_id: "span", name: "GET /orders", service_name: "checkout", status_code: "OK",
  started_at: "2026-10-01T00:00:00Z", ended_at: "2026-10-01T00:00:00.065Z",
  attributes: { "http.method": "GET", "http.status_code": 200, payload: { state: "queued" }, region: "local", attempt: 1 },
  resource_attributes: { "service.version": "1.0" }, events: [], links: [],
};

it("switches inspector tabs by keyboard and exposes the active panel", () => {
  render(<MemoryRouter><SpanDetail span={span} traceID="trace" startMs={0} durationMs={65} /></MemoryRouter>);
  const attributes = screen.getByRole("tab", { name: /Attributes/ });
  fireEvent.keyDown(attributes, { key: "ArrowRight" });
  const resource = screen.getByRole("tab", { name: /Resource/ });
  expect(resource.getAttribute("aria-selected")).toBe("true");
  expect(document.activeElement).toBe(resource);
  expect(screen.getByText("service.version")).toBeTruthy();
  fireEvent.keyDown(resource, { key: "End" });
  const raw = screen.getByRole("tab", { name: "Raw" });
  expect(raw.getAttribute("aria-selected")).toBe("true");
  expect(screen.getByRole("tabpanel").getAttribute("aria-labelledby")).toBe(raw.id);
});

it("searches structured attribute values and clears the filter", () => {
  render(<MemoryRouter><SpanDetail span={span} traceID="trace" startMs={0} durationMs={65} /></MemoryRouter>);
  fireEvent.change(screen.getByRole("textbox", { name: /Filter span attributes/ }), { target: { value: "QUEUED" } });
  expect(screen.getByText("payload")).toBeTruthy();
  expect(screen.queryByText("attempt")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Clear property filter" }));
  expect(screen.getByText("attempt")).toBeTruthy();
});
