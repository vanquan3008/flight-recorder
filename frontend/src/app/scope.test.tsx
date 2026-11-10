import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { App } from "./app";
import { emptyPerformance } from "../features/performance/test-data";

const projects = ["alpha", "beta"].map((slug) => ({ id: slug, slug, name: slug.toUpperCase(), created_at: "2026-01-01T00:00:00Z" }));
const view = {
  user: { id: "user", email: "audit@example.test", display_name: "Audit", platform_admin: false, must_change_password: false, created_at: "2026-01-01T00:00:00Z" },
  memberships: projects.map((project) => ({ project, role: "owner" })), access_token: "scope-jwt", expires_at: "2099-01-01T00:00:00Z",
};
afterEach(() => { cleanup(); localStorage.clear(); window.history.replaceState(null, "", "/"); vi.unstubAllGlobals(); });

function open(path: string) {
  localStorage.setItem("tracepilot-signed-in", "1");
  localStorage.setItem("tracepilot-project", "alpha");
  window.history.replaceState(null, "", path);
  const calls: { path: string; headers: Headers }[] = [];
  vi.stubGlobal("fetch", vi.fn((path: string, init: RequestInit = {}) => {
    calls.push({ path, headers: new Headers(init.headers) });
    const pathname = new URL(path, "http://localhost").pathname;
    let body: unknown = { data: [] };
    if (pathname === "/api/v1/auth/refresh") body = view;
    if (pathname === "/api/v1/performance") body = emptyPerformance;
    if (pathname === "/api/v1/telemetry/quality") body = { metric_points: new Headers(init.headers).get("X-Environment") === "production" ? 900 : 500 };
    if (pathname.startsWith("/api/v1/projects/")) body = { project: projects.find((p) => pathname.endsWith(p.id)), role: "owner", environments: [{ id: "env-dev", name: "development" }, { id: "env-prod", name: "production" }] };
    return Promise.resolve(new Response(JSON.stringify(body), { status: 200 }));
  }));
  render(<App />);
  return calls;
}

it("opens a shared URL in its project and environment on the first request", async () => {
  const calls = open("/logs?project=beta&environment=production&range=24h");
  await screen.findByRole("heading", { name: "Log explorer" });
  await waitFor(() => expect(calls.some((call) => call.path.startsWith("/api/v1/logs"))).toBe(true));
  for (const call of calls.filter((call) => !call.path.startsWith("/api/v1/auth/"))) {
    expect(call.headers.get("X-Project-ID"), call.path).toBe("beta");
    expect(call.headers.get("X-Environment"), call.path).toBe("production");
  }
  expect(screen.getByRole("combobox", { name: "Project" }).textContent).toContain("BETA");
  expect(screen.getByRole("combobox", { name: "Environment" }).textContent).toContain("production");
});

it("clears the old scope and keeps the window when changing environment and navigating", async () => {
  const calls = open("/logs?project=alpha&environment=development&range=24h&to=2026-10-02T00:00:00Z");
  await screen.findByRole("heading", { name: "Log explorer" });
  fireEvent.click(screen.getByRole("combobox", { name: "Environment" }));
  fireEvent.click(await screen.findByRole("option", { name: "production" }));
  await waitFor(() => expect(calls.some((call) => call.path.startsWith("/api/v1/logs") && call.headers.get("X-Environment") === "production")).toBe(true));
  fireEvent.click(screen.getByRole("link", { name: "Trace explorer" }));
  await screen.findByRole("heading", { name: "Trace explorer" });
  expect(window.location.search).toContain("environment=production");
  expect(window.location.search).toContain("range=24h");
  await waitFor(() => expect(calls.some((call) => call.path.startsWith("/api/v1/traces?") && call.path.includes("from="))).toBe(true));
  const trace = calls.filter((call) => call.path.startsWith("/api/v1/traces?") && call.path.includes("from=")).at(-1)!;
  expect(trace.headers.get("X-Environment")).toBe("production");
  expect(new URL(trace.path, "http://localhost").searchParams.get("from")).toBe("2026-10-01T00:00:00.000Z");
});

it("refuses an unknown project instead of showing another project's telemetry", async () => {
  const calls = open("/logs?project=not-a-member");
  await screen.findByText("You don't have access to this page");
  expect(calls.some((call) => call.path.startsWith("/api/v1/logs"))).toBe(false);
});

it("switches project from the dropdown and preserves the time window with its own environment", async () => {
  localStorage.setItem("tracepilot-environment:beta", "production");
  const calls = open("/logs?project=alpha&environment=development&range=24h&to=2026-10-02T00:00:00Z");
  await screen.findByRole("heading", { name: "Log explorer" });
  fireEvent.click(screen.getByRole("combobox", { name: "Project" }));
  fireEvent.click(screen.getByRole("option", { name: "BETA" }));
  await waitFor(() => expect(window.location.pathname).toBe("/overview"));
  expect(window.location.search).toContain("project=beta");
  expect(window.location.search).toContain("environment=production");
  expect(window.location.search).toContain("range=24h");
  await waitFor(() => expect(calls.some((call) => call.path.startsWith("/api/v1/performance") && call.headers.get("X-Project-ID") === "beta" && call.headers.get("X-Environment") === "production")).toBe(true));
});

it("does not reuse a fresh overview result from the previous environment", async () => {
  open("/overview?project=alpha&environment=development&range=24h&to=2026-10-02T00:00:00Z");
  await screen.findByText("500");
  fireEvent.click(screen.getByRole("combobox", { name: "Environment" }));
  fireEvent.click(await screen.findByRole("option", { name: "production" }));
  await screen.findByText("900");
  expect(screen.queryByText("500")).toBeNull();
});
