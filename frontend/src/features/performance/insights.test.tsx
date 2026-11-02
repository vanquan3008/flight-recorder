import { afterEach, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { QualityDonut, ServiceErrorBars } from "./insights";

afterEach(cleanup);

const stats = { requests: 100, errors: 0, requests_per_second: 1, error_rate: 0, average_latency_nanos: 50e6, p95_latency_nanos: 120e6, p99_latency_nanos: 200e6 };
const group = (service: string, overrides: Partial<typeof stats>) => ({ ...stats, service, ...overrides });
const renderBars = (services: ReturnType<typeof group>[]) => render(<MemoryRouter><ServiceErrorBars services={services} from="2026-10-01T00:00:00Z" to="2026-10-01T01:00:00Z" /></MemoryRouter>);

it("ranks services by errors and scales each bar to the worst one", () => {
  renderBars([group("catalog", { errors: 1, error_rate: 0.01 }), group("checkout", { errors: 8, error_rate: 0.08 }), group("search", { errors: 0 })]);
  const rows = screen.getAllByRole("listitem");
  expect(rows.map((row) => row.querySelector("a")?.textContent)).toEqual(["checkout", "catalog", "search"]);
  const bar = (name: RegExp) => screen.getByRole("img", { name }).querySelector<HTMLElement>(".bar-fill");
  expect(bar(/^checkout/)?.style.width).toBe("100%");
  expect(bar(/^catalog/)?.style.width).toBe("12.5%");
  expect(bar(/^search/)).toBeNull();
  expect(screen.getByRole("link", { name: "checkout" }).getAttribute("href")).toContain("service=checkout");
});

it("says so when no service had errors, and when there is no traffic", () => {
  const { unmount } = renderBars([group("checkout", {})]);
  expect(screen.getByText("No request errors in this window.")).toBeTruthy();
  unmount();
  renderBars([]);
  expect(screen.getByText(/No inbound requests in this window/)).toBeTruthy();
});

it("shows how many logs carry a trace and how many spans find their parent", () => {
  render(<QualityDonut quality={{ from: "", to: "", spans: 200, orphan_spans: 50, logs: 80, uncorrelated_logs: 20, metric_points: 0 }} />);
  expect(screen.getByRole("img", { name: "75% of logs carry a trace" })).toBeTruthy();
  expect(screen.getByText("60 of 80")).toBeTruthy();
  expect(screen.getByText("150 of 200")).toBeTruthy();
});

it("does not invent a percentage for an empty window", () => {
  render(<QualityDonut quality={{ from: "", to: "", spans: 0, orphan_spans: 0, logs: 0, uncorrelated_logs: 0, metric_points: 0 }} />);
  expect(screen.getByText("No logs or spans in this window.")).toBeTruthy();
  expect(document.body.textContent).not.toMatch(/NaN|Infinity/);
});
