import { afterEach, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { emptyPerformance } from "../performance/test-data";
import type { Incident } from "../../shared/types";
import { StatusHero, overviewStatus, trafficSummary } from "./hero";

afterEach(cleanup);

const busy = { ...emptyPerformance, summary: { requests: 1200, errors: 12, requests_per_second: 2, error_rate: 0.01, average_latency_nanos: 50e6, p95_latency_nanos: 230e6, p99_latency_nanos: 400e6 } };
const incident = (id: string, title: string) => ({ id, title, severity: "HIGH", status: "OPEN", primary_service: "checkout" }) as Incident;

it("leads with the worst thing to know: incidents, then failing traces, then calm, then silence", () => {
  expect(overviewStatus({ openIncidents: 2, failedTraces: 5, performance: busy })).toMatchObject({ tone: "danger", headline: "2 incidents need attention" });
  expect(overviewStatus({ openIncidents: 1, failedTraces: 0 }).headline).toBe("1 incident needs attention");
  expect(overviewStatus({ openIncidents: 0, failedTraces: 3, performance: busy })).toMatchObject({ tone: "warning", headline: "Errors observed in recent traces" });
  expect(overviewStatus({ openIncidents: 0, failedTraces: 0, performance: busy })).toMatchObject({ tone: "warning", headline: "Errors observed in this window" });
  expect(overviewStatus({ openIncidents: 0, failedTraces: 0, performance: emptyPerformance }).tone).toBe("idle");
  expect(overviewStatus({ openIncidents: 0, failedTraces: 0 }).tone).toBe("idle");
});

it("does not claim overall health from a partial incident list or an unavailable traffic query", () => {
  const calm = { ...busy, summary: { ...busy.summary, errors: 0, error_rate: 0 } };
  expect(overviewStatus({ openIncidents: 0, failedTraces: 0, performance: calm, incidentsIncomplete: true }).tone).toBe("warning");
  expect(overviewStatus({ openIncidents: 0, failedTraces: 0 }).headline).toBe("Traffic status is unavailable");
  expect(overviewStatus({ openIncidents: 0, failedTraces: 0, performance: calm }).headline).toBe("No request errors observed");
});

it("summarises traffic only when there was some", () => {
  expect(trafficSummary(busy)).toBe("1,200 requests · 1.0% errors · p95 230.0ms");
  expect(trafficSummary(emptyPerformance)).toBe("");
  expect(trafficSummary(undefined)).toBe("");
});

it("lists the first three incidents with a way to open each and the rest", () => {
  const incidents = ["a", "b", "c", "d"].map((id) => incident(id, `Incident ${id}`));
  render(<MemoryRouter><StatusHero status={overviewStatus({ openIncidents: 4, failedTraces: 0, performance: busy })} incidents={incidents} performance={busy} pending={false} filters="range=15m" /></MemoryRouter>);
  expect(screen.getByRole("heading", { name: "4 incidents need attention" })).toBeTruthy();
  expect(screen.getAllByRole("link", { name: "Investigate" }).map((link) => link.getAttribute("href"))).toEqual(["/incidents/a", "/incidents/b", "/incidents/c"]);
  expect(screen.getByRole("link", { name: /All 4 incidents/ }).getAttribute("href")).toContain("/incidents?range=15m");
});

it("offers the traces instead when nothing needs attention, and never shows NaN", () => {
  render(<MemoryRouter><StatusHero status={overviewStatus({ openIncidents: 0, failedTraces: 0, performance: emptyPerformance })} incidents={[]} performance={emptyPerformance} pending={false} filters="range=15m" /></MemoryRouter>);
  expect(screen.getByRole("link", { name: "Explore traces →" })).toBeTruthy();
  expect(screen.queryByRole("link", { name: "Investigate" })).toBeNull();
  expect(document.body.textContent).not.toMatch(/NaN|Infinity/);
});
