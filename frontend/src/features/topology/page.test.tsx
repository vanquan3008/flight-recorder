import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { App } from "../../app/app";

const respond = (data: unknown) => new Response(JSON.stringify(data), { status: 200 });
const edge = (source: string, target: string, kind: string) => ({ source_node_id: source, target_node_id: target, kind, call_count: 3, error_count: 1, average_latency_nanos: 9e8, p95_latency_nanos: 9e8, trace_ids: [] });
const topology = {
  nodes: [{ id: "service:checkout", type: "SERVICE", name: "checkout" }, { id: "service:payment", type: "SERVICE", name: "payment" }, { id: "external:provider", type: "EXTERNAL", name: "provider" }],
  edges: [edge("service:checkout", "service:payment", "service"), edge("service:payment", "external:provider", "external")],
};
const logs = [{ event_id: "e1", occurred_at: "2026-10-01T00:00:01Z", service_name: "payment", severity_text: "ERROR", body: { message: "card declined" } }];

afterEach(() => { cleanup(); sessionStorage.clear(); window.history.pushState(null, "", "/"); window.location.hash = ""; vi.unstubAllGlobals(); });

function open(hash = "#/topology") {
  localStorage.setItem("tracepilot-signed-in", "1");
  const path = hash.startsWith("#") ? hash.slice(1) : hash;
  window.history.pushState(null, "", path);
  window.location.hash = "";
  const fetch = vi.fn().mockImplementation((input: string) => {
    const pathname = new URL(input, "http://localhost").pathname;
    return Promise.resolve(respond(pathname === "/api/v1/topology" ? topology : pathname === "/api/v1/logs" ? { data: logs } : { data: [] }));
  });
  vi.stubGlobal("fetch", fetch);
  render(<App />);
  return fetch;
}

it("lists services and shows the flow position and recent logs of the selected one", async () => {
  const fetch = open();
  fireEvent.click(await screen.findByRole("button", { name: /^payment service/i }));
  const flow = await screen.findByLabelText("Where payment sits in the flow");
  expect(within(flow).getByRole("button", { name: /checkout/ })).toBeTruthy();
  expect(within(flow).getByRole("button", { name: /provider/ })).toBeTruthy();
  expect(await screen.findByText(/card declined/)).toBeTruthy();
  const logRequest = fetch.mock.calls.map(([path]) => new URL(String(path), "http://localhost")).find((url) => url.pathname === "/api/v1/logs" && url.searchParams.get("service") === "payment");
  expect(logRequest?.searchParams.get("service")).toBe("payment");
  expect(logRequest?.searchParams.get("limit")).toBe("20");
  expect(screen.getByRole("link", { name: /Open in Log explorer/ }).getAttribute("href")).toContain("service=payment");
});

it("keeps the full topology window when opening and searching its logs", async () => {
  const fetch = open("#/topology?range=24h&to=2026-10-02T00:00:00Z");
  await screen.findByRole("link", { name: /Open in Log explorer/ });
  fireEvent.click(screen.getByRole("link", { name: /Open in Log explorer/ }));
  await screen.findByRole("heading", { name: "Log explorer" });
  fireEvent.change(screen.getByLabelText("Search log body"), { target: { value: "card" } });
  fireEvent.click(screen.getByRole("button", { name: "Search logs" }));
  await waitFor(() => {
    const requests = fetch.mock.calls.map(([path]) => new URL(String(path), "http://localhost"));
    const search = requests.find((url) => url.pathname === "/api/v1/logs" && url.searchParams.get("search") === "card");
    expect(search?.searchParams.get("from")).toBe("2026-10-01T00:00:00.000Z");
    expect(search?.searchParams.get("to")).toBe("2026-10-02T00:00:00.000Z");
  });
});

it("follows a neighbour in the flow and skips logs for a dependency that writes none", async () => {
  const fetch = open();
  fireEvent.click(await screen.findByRole("button", { name: /^checkout/ }));
  const flow = await screen.findByLabelText("Where checkout sits in the flow");
  fireEvent.click(within(flow).getByRole("button", { name: /payment/ }));
  await screen.findByLabelText("Where payment sits in the flow");
  fireEvent.click(within(screen.getByLabelText("Where payment sits in the flow")).getByRole("button", { name: /provider/ }));
  expect(await screen.findByText(/dependencies do not write logs/)).toBeTruthy();
  await waitFor(() => expect(fetch.mock.calls.filter(([path]) => String(path).startsWith("/api/v1/logs")).every(([path]) => !String(path).includes("service=provider"))).toBe(true));
});

it("switches between the list and the graph and can always go back", async () => {
  open();
  await screen.findByRole("button", { name: /^checkout service/i });
  expect(document.querySelector(".service-list")).not.toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Graph" }));
  await waitFor(() => expect(window.location.search).toContain("view=graph"));
  expect(screen.getByRole("region", { name: "Service topology graph" })).toBeTruthy();
  expect(document.querySelector(".service-list")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "List" }));
  await waitFor(() => expect(window.location.search).not.toContain("view="));
  expect(await screen.findByRole("button", { name: /^checkout service/i })).toBeTruthy();
  expect(document.querySelector(".service-list")).not.toBeNull();
});

it("keeps the graph in one panel: nothing is open until a service is selected, and the detail opens beside it", async () => {
  const fetch = open("#/topology?view=graph");
  await screen.findByRole("region", { name: "Service topology graph" });
  expect(screen.queryByRole("complementary")).toBeNull();
  expect(document.querySelector(".graph-node.dim")).toBeNull();
  expect(document.querySelectorAll(".panel")).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: /^SERVICE checkout/ }));
  const drawer = await screen.findByRole("complementary", { name: "Details for checkout" });
  expect(within(drawer).getByRole("button", { name: "Calls payment" })).toBeTruthy();
  expect(await within(drawer).findByText(/card declined/)).toBeTruthy();
  expect(document.querySelectorAll(".panel")).toHaveLength(1);
  const logRequest = fetch.mock.calls.map(([path]) => new URL(String(path), "http://localhost")).find((url) => url.pathname === "/api/v1/logs");
  expect(logRequest?.searchParams.get("service")).toBe("checkout");
  fireEvent.click(within(drawer).getByRole("button", { name: "Calls payment" }));
  expect(await screen.findByRole("complementary", { name: "Details for payment" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Close detail" }));
  expect(screen.queryByRole("complementary")).toBeNull();
});

it("explains that a dependency has no logs instead of querying for them", async () => {
  const fetch = open("#/topology?view=graph");
  fireEvent.click(await screen.findByRole("button", { name: /^EXTERNAL provider/ }));
  const drawer = await screen.findByRole("complementary", { name: "Details for provider" });
  expect(within(drawer).getByText(/dependencies do not write logs/)).toBeTruthy();
  expect(fetch.mock.calls.some(([path]) => String(path).includes("service=provider"))).toBe(false);
});
