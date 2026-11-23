import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { Waterfall } from "./waterfall";
import { buildTrace } from "./trace-model";

const base = Date.parse("2026-10-01T00:00:00Z");
afterEach(cleanup);
const span = (id: string, start: number, end: number, parentId?: string) => ({
  trace_id: "trace1",
  span_id: id,
  parent_span_id: parentId,
  name: `span-${id}`,
  service_name: "test-svc",
  status_code: "OK",
  started_at: new Date(base + start).toISOString(),
  ended_at: new Date(base + end).toISOString(),
  attributes: {},
  resource_attributes: {},
  events: [],
  links: [],
});

describe("Waterfall virtualized rendering", () => {
  it("finds errors inside a collapsed subtree and searches by operation", () => {
    const model = buildTrace([span("root", 0, 100), { ...span("failed", 10, 30, "root"), status_code: "ERROR" }, span("child", 40, 60, "root")]);
    const { container } = render(<MemoryRouter><Waterfall model={model} traceID="trace1" focus="" onFocus={() => {}} /></MemoryRouter>);
    fireEvent.click(screen.getByRole("button", { name: "Collapse all" }));
    expect(container.querySelectorAll(".wf-row")).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: /^Errors/ }));
    expect(container.querySelectorAll(".wf-row")).toHaveLength(1);
    expect(screen.getByText("span-failed")).toBeTruthy();
    fireEvent.change(screen.getByRole("searchbox", { name: "Search spans" }), { target: { value: "no match" } });
    expect(screen.getByText("No spans match these filters.")).toBeTruthy();
  });
  it("renders empty state when trace has no spans", () => {
    const model = buildTrace([]);
    render(<MemoryRouter><Waterfall model={model} traceID="trace1" focus="" onFocus={() => {}} /></MemoryRouter>);
    expect(screen.getByText("This trace has no spans.")).toBeTruthy();
  });

  it("handles many spans gracefully", () => {
    const spans = [
      span("root", 0, 500),
      ...Array.from({ length: 49 }, (_, i) => span(`child-${i}`, i * 10, i * 10 + 5, "root")),
    ];
    const model = buildTrace(spans);
    const { container } = render(<MemoryRouter><Waterfall model={model} traceID="trace1" focus="" onFocus={() => {}} /></MemoryRouter>);
    expect(screen.getAllByText("test-svc").length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "Collapse all" })).toBeTruthy();
    expect(container.querySelector(".wf-viewport")).not.toBeNull();
  });
});
