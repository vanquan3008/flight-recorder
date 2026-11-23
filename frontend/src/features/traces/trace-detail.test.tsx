import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { App } from "../../app/app";

const traceID = "0123456789abcdef0123456789abcdef";
const base = Date.parse("2026-10-01T00:00:00Z");
const span = (id: string, service: string, name: string, start: number, end: number, extra: Record<string, unknown> = {}) => ({
  trace_id: traceID, span_id: id, name, service_name: service, status_code: "OK", started_at: new Date(base + start).toISOString(), ended_at: new Date(base + end).toISOString(),
  attributes: {}, resource_attributes: {}, events: [], links: [], ...extra,
});
const waterfallRows = () => [...document.querySelectorAll<HTMLElement>(".wf-rows > li")];
const spans = [
  span("gw", "gateway", "GET /api/orders/123", 0, 250),
  span("order", "order-service", "GET /orders/{id}", 20, 230, { parent_span_id: "gw" }),
  span("db", "postgres", "SELECT orders", 40, 215, { parent_span_id: "order" }),
  span("fail", "inventory", "POST /reserve", 216, 240, { parent_span_id: "order", status_code: "ERROR" }),
];

afterEach(() => { cleanup(); sessionStorage.clear(); window.location.hash = ""; vi.unstubAllGlobals(); });

function open(data = spans) {
  localStorage.setItem("tracepilot-signed-in", "1");
  window.location.hash = `#/traces/${traceID}`;
  vi.stubGlobal("fetch", vi.fn().mockImplementation((input: string) => {
    const path = new URL(input, "http://localhost").pathname;
    return new Response(JSON.stringify(path === `/api/v1/traces/${traceID}` ? { trace_id: traceID, spans: data } : { data: [] }), { status: 200 });
  }));
  render(<App />);
}

it("summarises the whole request and shows who called whom", async () => {
  open();
  expect((await screen.findAllByText("250 ms")).length).toBeGreaterThanOrEqual(1);
  const flow = (await screen.findByRole("heading", { name: "Request flow" })).closest("section")!;
  const stats = Object.fromEntries([...document.querySelectorAll(".flow-stats > div")].map((item) => [item.querySelector("span")!.textContent, item.querySelector("strong")!.textContent]));
  expect(stats).toEqual({ "Total duration": "250 ms", Spans: "4", Services: "4", Errors: "1" });
  expect(within(flow).queryByRole("region", { name: "Service flow for this trace" })).toBeNull();
  fireEvent.click(within(flow).getByRole("button", { name: "Service map" }));
  expect(within(flow).getByRole("region", { name: "Service flow for this trace" })).toBeTruthy();
  const hops = within(flow).getAllByRole("row").map((row) => row.textContent);
  expect(hops.some((text) => text?.startsWith("order-servicepostgres1"))).toBe(true);
  expect(hops.some((text) => text?.startsWith("order-serviceinventory11"))).toBe(true);
});

it("lays spans out as a time-scaled waterfall with an axis and marks failures in text", async () => {
  open();
  await screen.findByRole("heading", { name: "Span waterfall" });
  const rows = waterfallRows();
  expect(rows.map((row) => within(row).getByText(/^(GET|SELECT|POST)/).textContent)).toEqual(["GET /api/orders/123", "GET /orders/{id}", "SELECT orders", "POST /reserve"]);
  expect(within(rows[3]).getByText("Error")).toBeTruthy();
  const bar = rows[2].querySelector(".wf-bar") as HTMLElement;
  expect(parseFloat(bar.style.left)).toBeCloseTo(16, 0);
  expect(parseFloat(bar.style.width)).toBeCloseTo(70, 0);
  expect(document.querySelector(".wf-axis")?.textContent).toContain("250 ms");
});

it("collapses and expands a subtree and opens a separate span inspector", async () => {
  open();
  fireEvent.click(await screen.findByRole("button", { name: "Collapse GET /orders/{id}" }));
  expect(screen.queryByText("SELECT orders")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Expand all" }));
  fireEvent.click(await screen.findByText("SELECT orders"));
  const detail = screen.getByRole("region", { name: "Span SELECT orders" });
  expect(detail.closest(".wf-inspector")).not.toBeNull();
  expect(within(detail).getByText(/starts at \+40\.0 ms/)).toBeTruthy();
  expect(within(detail).getByRole("link", { name: "Logs for this span" }).getAttribute("href")).toContain("spanId=db");
});

it("highlights one service's spans from the flow and clears the focus again", async () => {
  open();
  const share = (await screen.findByRole("heading", { name: /Where the time went/ })).closest("div")!;
  fireEvent.click(within(share).getByRole("button", { name: /postgres/ }));
  const rows = waterfallRows();
  expect(rows.filter((row) => row.className.includes("dim")).length).toBe(3);
  expect(rows[2].className).not.toContain("dim");
  fireEvent.click(within(share).getByRole("button", { name: /postgres/ }));
  expect(waterfallRows().some((row) => row.className.includes("dim"))).toBe(false);
});

it("explains a single-service trace instead of drawing an empty graph", async () => {
  open([span("a", "api", "GET /health", 0, 5), span("b", "api", "db.query", 1, 3, { parent_span_id: "a" })]);
  expect(await screen.findByText("Single-service trace")).toBeTruthy();
});

it("keeps rendering a trace with no spans", async () => {
  open([]);
  expect(await screen.findByText("This trace has no spans.")).toBeTruthy();
});

it("shows correlated logs when the trace API returns 404", async () => {
  localStorage.setItem("tracepilot-signed-in", "1");
  window.location.hash = "#/traces/" + traceID;
  vi.stubGlobal("fetch", vi.fn().mockImplementation((input: string) => {
    const path = new URL(input, "http://localhost").pathname;
    if (path === "/api/v1/traces/" + traceID) return new Response("{}", { status: 404 });
    const data = path.endsWith("/logs") ? [{
      event_id: "log-1", trace_id: traceID, service_name: "tracepilot-ai-demo",
      span_id: "0123456789abcdef", body: "policy.retrieved", severity_text: "INFO",
      occurred_at: new Date(base).toISOString(),
    }] : [];
    return new Response(JSON.stringify({ data }), { status: 200 });
  }));
  render(<App />);
  expect(await screen.findByText("Trace not available")).toBeTruthy();
  expect((await screen.findAllByText(/policy\.retrieved/)).length).toBeGreaterThan(0);
  expect(screen.getByRole("link", { name: /Open Log explorer/ }).getAttribute("href")).toContain("traceId=" + traceID);
});

it("shows logs before the trace arrives and keeps the log filter when it loads", async () => {
  localStorage.setItem("tracepilot-signed-in", "1");
  window.location.hash = "#/traces/" + traceID;
  let completeTrace!: (response: Response) => void;
  const pendingTrace = new Promise<Response>((resolve) => { completeTrace = resolve; });
  vi.stubGlobal("fetch", vi.fn().mockImplementation((input: string) => {
    const path = new URL(input, "http://localhost").pathname;
    if (path === "/api/v1/traces/" + traceID) return pendingTrace;
    return new Response(JSON.stringify({ data: path.endsWith("/logs") ? [{
      event_id: "log-1", trace_id: traceID, service_name: "tracepilot-ai-demo",
      body: "policy.retrieved", severity_text: "INFO", occurred_at: new Date(base).toISOString(),
    }] : [] }), { status: 200 });
  }));
  render(<App />);
  expect((await screen.findAllByText(/policy\.retrieved/)).length).toBeGreaterThan(0);
  expect(screen.getByRole("heading", { name: "Loading trace spans" })).toBeTruthy();
  fireEvent.change(screen.getByLabelText("Filter loaded logs"), { target: { value: "request" } });
  completeTrace(new Response(JSON.stringify({ trace_id: traceID, spans }), { status: 200 }));
  await screen.findByRole("heading", { name: "Span waterfall" });
  expect((screen.getByLabelText("Filter loaded logs") as HTMLInputElement).value).toBe("request");
  expect(screen.getByText("No loaded logs match this filter.")).toBeTruthy();
});
