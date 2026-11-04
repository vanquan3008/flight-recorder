import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { Waterfall } from "./waterfall";
import { buildTrace } from "./trace-model";

const base = Date.parse("2026-10-01T00:00:00Z");
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
  it("renders empty state when trace has no spans", () => {
    const model = buildTrace([]);
    render(<Waterfall model={model} traceID="trace1" focus="" onFocus={() => {}} />);
    expect(screen.getByText("This trace has no spans.")).toBeTruthy();
  });

  it("handles many spans gracefully", () => {
    const spans = [
      span("root", 0, 500),
      ...Array.from({ length: 49 }, (_, i) => span(`child-${i}`, i * 10, i * 10 + 5, "root")),
    ];
    const model = buildTrace(spans);
    const { container } = render(<Waterfall model={model} traceID="trace1" focus="" onFocus={() => {}} />);
    expect(screen.getAllByText("test-svc").length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "Collapse all" })).toBeTruthy();
    expect(container.querySelector(".wf-viewport")).not.toBeNull();
  });
});
