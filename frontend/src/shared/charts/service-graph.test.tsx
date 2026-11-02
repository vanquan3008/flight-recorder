import { afterEach, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { useState } from "react";
import { GraphLegend, ServiceGraph } from "./service-graph";
import type { TopologyEdge, TopologyNode } from "../types";

afterEach(cleanup);

const nodes: TopologyNode[] = [{ id: "s:web", type: "service", name: "web" }, { id: "s:api", type: "service", name: "api" }, { id: "e:db", type: "database", name: "db" }, { id: "s:idle", type: "service", name: "idle" }];
const edge = (source: string, target: string, calls: number, errors: number): TopologyEdge => ({ source_node_id: source, target_node_id: target, kind: "calls", call_count: calls, error_count: errors, average_latency_nanos: 45e6, p95_latency_nanos: 90e6, trace_ids: [] });
const edges = [edge("s:web", "s:api", 12, 2), edge("s:api", "e:db", 8, 0)];

function Harness() {
  const [selected, setSelected] = useState("");
  return <ServiceGraph nodes={nodes} edges={edges} selected={selected} onSelect={setSelected} label="graph" />;
}

it("shows each service's traffic and its errors on the card", () => {
  const { container } = render(<Harness />);
  const web = screen.getByRole("button", { name: /^service web/ });
  expect(within(web).getByText("in 0 · out 12")).toBeTruthy();
  expect(within(web).getByText("2 err")).toBeTruthy();
  expect(within(screen.getByRole("button", { name: /^service api/ })).getByText("in 12 · out 8")).toBeTruthy();
  expect(within(screen.getByRole("button", { name: /^service api/ })).queryByText(/ err$/)).toBeNull();
  expect(within(screen.getByRole("button", { name: /^service idle/ })).getByText("No observed calls")).toBeTruthy();
  // A call's numbers stay out of the way until it is hovered or its service is focused.
  expect(container.querySelector(".graph-label")).toBeNull();
});

it("shows a call's numbers while it is hovered", () => {
  const { container } = render(<Harness />);
  fireEvent.mouseEnter(container.querySelectorAll(".graph-hit")[1]);
  expect(screen.getByText("8 calls")).toBeTruthy();
  expect(screen.getByText("avg 45.0ms")).toBeTruthy();
  expect(screen.queryByText("12 calls")).toBeNull();
});

it("focuses a service: unrelated nodes dim and its calls gain latency", () => {
  render(<Harness />);
  fireEvent.click(screen.getByRole("button", { name: /^service web/ }));
  const state = (name: RegExp) => screen.getByRole("button", { name }).getAttribute("class") ?? "";
  expect(state(/^service web/)).toContain("selected");
  expect(state(/^service api/)).not.toContain("dim");
  expect(state(/^database db/)).toContain("dim");
  expect(screen.getByText("12 calls")).toBeTruthy();
  expect(screen.getByText("avg 45.0ms")).toBeTruthy();
  expect(screen.queryByText("8 calls")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: /^service web/ }));
  expect(state(/^database db/)).not.toContain("dim");
});

it("lists only the node types that are drawn", () => {
  render(<GraphLegend nodes={nodes} />);
  expect(screen.getByText("database")).toBeTruthy();
  expect(screen.getByText("service")).toBeTruthy();
  expect(screen.queryByText("messaging")).toBeNull();
  expect(screen.getByText("Calls with errors")).toBeTruthy();
});
