import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { App } from "./app";
import { emptyPerformance } from "../features/performance/test-data";
import { setAccessToken, setProjectContext } from "../shared/api";

afterEach(() => {
  cleanup();
  sessionStorage.clear();
  localStorage.clear();
  window.location.hash = "";
  vi.unstubAllGlobals();
});

describe("App", () => {
  it("keeps protected routes behind an account, with registration beside sign-in", () => {
    render(<App />);
    expect(screen.getByRole("heading", { name: "Welcome to TracePilot" })).toBeTruthy();
    expect(screen.getByLabelText("Email")).toBeTruthy();
    expect(screen.getByLabelText("Password")).toBeTruthy();
    expect(screen.queryByLabelText(/API key/i)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Register" }));
    expect(screen.getByLabelText("Display name")).toBeTruthy();
  });

  it("guides a connected operator from an empty console to the first trace", async () => {
    localStorage.setItem("tracepilot-signed-in", "1");
    window.location.hash = "#/overview";
    vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string) => new Response(JSON.stringify(path.startsWith("/api/v1/performance") ? emptyPerformance : { data: [] }), { status: 200 })));
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Get your first trace" })).toBeTruthy();
    expect(screen.getByText(/Configure OTLP/)).toBeTruthy();
    expect(screen.getByText("Send a request from your instrumented application, then refresh this page.")).toBeTruthy();
  });

  it("signs an account in, works inside its project with a bearer token, and signs out", async () => {
    const project = { id: "01900000-0000-7000-8000-0000000000a1", slug: "alpha", name: "Alpha", created_at: "2026-01-01T00:00:00Z" };
    const view = {
      user: { id: "u1", email: "alice@example.com", display_name: "Alice", platform_admin: false, must_change_password: false, created_at: "2026-01-01T00:00:00Z" },
      memberships: [{ project, role: "user" }], access_token: "jwt-1", expires_at: "2099-01-01T00:00:00Z",
    };
    const calls: { path: string; init: RequestInit }[] = [];
    vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init: RequestInit = {}) => {
      calls.push({ path, init });
      if (path === "/api/v1/auth/status") return new Response(JSON.stringify({ registration_open: true, first_account: false }), { status: 200 });
      if (path === "/api/v1/auth/login") return new Response(JSON.stringify(view), { status: 200 });
      if (path === "/api/v1/auth/logout") return new Response(null, { status: 204 });
      return new Response(JSON.stringify(path.startsWith("/api/v1/performance") ? emptyPerformance : { data: [] }), { status: 200 });
    }));
    render(<App />);
    fireEvent.change(await screen.findByLabelText("Email"), { target: { value: "alice@example.com" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "a-long-enough-passphrase" } });
    fireEvent.click(screen.getAllByRole("button", { name: /^Sign in/ }).at(-1)!);

    expect(await screen.findByRole("button", { name: /Sign out/ })).toBeTruthy();
    expect(screen.getByRole("combobox", { name: "Project" }).textContent).toContain(project.name);
    await waitFor(() => expect(calls.some((call) => call.path.startsWith("/api/v1/traces"))).toBe(true));
    const read = calls.find((call) => call.path.startsWith("/api/v1/traces"))!;
    const headers = new Headers(read.init.headers);
    expect(headers.get("Authorization")).toBe("Bearer jwt-1");
    expect(headers.get("X-Project-ID")).toBe(project.id);
    expect(read.init.credentials).toBeUndefined();
    expect(localStorage.getItem("tracepilot-signed-in")).toBe("1");
    expect(JSON.stringify(Object.entries(localStorage)) + JSON.stringify(Object.entries(sessionStorage))).not.toContain("jwt-1");

    fireEvent.click(screen.getByRole("button", { name: /Sign out/ }));
    expect(await screen.findByRole("heading", { name: "Welcome to TracePilot" })).toBeTruthy();
    expect(calls.some((call) => call.path === "/api/v1/auth/logout" && call.init.credentials === "include")).toBe(true);
    expect(localStorage.getItem("tracepilot-signed-in")).toBeNull();
  });

  it("sends the project on every call a page makes right after the session resumes", async () => {
    // Module-level request context outlives a test; start from none so a leftover project cannot hide a late update.
    setAccessToken("");
    setProjectContext({ project: "" });
    localStorage.setItem("tracepilot-signed-in", "1");
    window.location.hash = "#/incidents";
    const calls: { path: string; headers: Headers }[] = [];
    vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init: RequestInit = {}) => {
      calls.push({ path, headers: new Headers(init.headers) });
      return new Response(JSON.stringify({ data: [] }), { status: 200 });
    }));
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Incidents" })).toBeTruthy();
    await waitFor(() => expect(calls.some((call) => call.path.startsWith("/api/v1/incidents"))).toBe(true));
    const projectCalls = calls.filter((call) => !call.path.startsWith("/api/v1/auth/"));
    expect(projectCalls.length).toBeGreaterThan(0);
    for (const call of projectCalls) expect(call.headers.get("X-Project-ID"), call.path).toBe("01900000-0000-7000-8000-0000000000a1");
  });

  it("registers a new account and enters the console", async () => {
    const view = {
      user: { id: "u9", email: "new@example.com", display_name: "New", platform_admin: false, must_change_password: false, created_at: "2026-01-01T00:00:00Z" },
      memberships: [], access_token: "jwt-9", expires_at: "2099-01-01T00:00:00Z",
    };
    let registered: unknown;
    vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init: RequestInit = {}) => {
      if (path === "/api/v1/auth/status") return new Response(JSON.stringify({ registration_open: true, first_account: false }), { status: 200 });
      if (path === "/api/v1/auth/register") { registered = JSON.parse(String(init.body)); return new Response(JSON.stringify(view), { status: 201 }); }
      return new Response(JSON.stringify({ data: [] }), { status: 200 });
    }));
    render(<App />);
    fireEvent.click(screen.getByRole("button", { name: "Register" }));
    fireEvent.change(screen.getByLabelText("Display name"), { target: { value: "New" } });
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "new@example.com" } });
    fireEvent.change(screen.getByLabelText(/^Password/), { target: { value: "a-long-enough-passphrase" } });
    fireEvent.click(screen.getByRole("button", { name: /Create account/ }));
    // A new account belongs to no project yet, so the console offers to make the first one.
    expect(await screen.findByRole("heading", { name: "Create your first project" })).toBeTruthy();
    expect(registered).toEqual({ email: "new@example.com", display_name: "New", password: "a-long-enough-passphrase" });
  });

  it("holds an account with a temporary password at the password change", async () => {
    const view = {
      user: { id: "u1", email: "bob@example.com", display_name: "Bob", platform_admin: false, must_change_password: true, created_at: "2026-01-01T00:00:00Z" },
      memberships: [{ project: { id: "p1", slug: "alpha", name: "Alpha", created_at: "2026-01-01T00:00:00Z" }, role: "user" }],
      access_token: "jwt-2", expires_at: "2099-01-01T00:00:00Z",
    };
    const fetchMock = vi.fn().mockImplementation((path: string) => new Response(JSON.stringify(path === "/api/v1/auth/refresh" ? view : { data: [] }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    localStorage.setItem("tracepilot-signed-in", "1");
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Choose your own password" })).toBeTruthy();
    expect(fetchMock.mock.calls.some(([path]) => String(path).startsWith("/api/v1/traces"))).toBe(false);
  });

  it("shows the product flow without requiring an API key", async () => {
    window.location.hash = "#/flow";
    render(<App />);

    expect(await screen.findByRole("heading", { name: /From signal to answer/ })).toBeTruthy();
    expect(screen.getByText("OTLP ingestion API")).toBeTruthy();
    expect(screen.getByText("Management REST API")).toBeTruthy();
    expect(screen.getByText("Incident and RCA")).toBeTruthy();
  });
});
