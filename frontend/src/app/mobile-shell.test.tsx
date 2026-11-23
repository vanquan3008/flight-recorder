import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { App } from "./app";
import { emptyPerformance } from "../features/performance/test-data";
import { mobileQuery } from "../shared/ui";

const alpha = { id: "01900000-0000-7000-8000-0000000000a1", slug: "alpha", name: "Alpha", created_at: "2026-01-01T00:00:00Z" };
const beta = { id: "01900000-0000-7000-8000-0000000000b2", slug: "beta", name: "Beta", created_at: "2026-01-01T00:00:00Z" };
const session = {
  user: { id: "u1", email: "owner@example.com", display_name: "Test Owner", platform_admin: false, must_change_password: false, created_at: "2026-01-01T00:00:00Z" },
  memberships: [{ project: alpha, role: "owner" }, { project: beta, role: "owner" }], access_token: "jwt", expires_at: "2099-01-01T00:00:00Z",
};

function stubServer() {
  const fetchMock = vi.fn().mockImplementation((path: string) => {
    const url = new URL(path, "http://localhost");
    let body: unknown = { data: [] };
    if (url.pathname === "/api/v1/auth/refresh") body = session;
    else if (url.pathname.startsWith("/api/v1/projects/")) body = { project: alpha, role: "owner", environments: [{ id: "env-p", name: "production" }, { id: "env-s", name: "staging" }] };
    else if (url.pathname.startsWith("/api/v1/performance")) body = emptyPerformance;
    return new Response(JSON.stringify(body), { status: 200 });
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function openOnPhone(path: string) {
  localStorage.setItem("tracepilot-signed-in", "1");
  window.history.pushState(null, "", path);
  render(<App />);
}

beforeEach(() => {
  vi.stubGlobal("matchMedia", (query: string) => ({ matches: query === mobileQuery, addEventListener() {}, removeEventListener() {} }));
});
afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.unstubAllGlobals();
  window.history.pushState(null, "", "/");
});

describe("console on a phone", () => {
  it("navigates with a bottom bar and keeps the remaining sections and the account under More", async () => {
    stubServer();
    openOnPhone(`/overview?project=${alpha.id}`);
    await screen.findByRole("heading", { name: "Get your first trace" }); // the lazy page has loaded
    const bar = screen.getByRole("navigation", { name: "Console sections" });
    expect(within(bar).getAllByRole("link").map((link) => link.textContent)).toEqual(["Overview", "Traces", "Logs", "Incidents"]);
    expect(screen.queryByRole("button", { name: "Toggle navigation" })).toBeNull();
    expect(screen.queryByRole("button", { name: /sidebar/i })).toBeNull();

    const more = within(bar).getByRole("button", { name: "More" });
    more.focus(); // a keyboard user opens the sheet from the focused button
    fireEvent.click(more);
    const sheet = screen.getByRole("dialog", { name: "More" });
    expect(within(sheet).getAllByRole("link").map((link) => link.textContent)).toEqual(["Performance", "Service topology", "Settings", "How it works"]);
    expect(within(sheet).getByRole("group", { name: "Color theme" })).toBeTruthy();
    expect(within(sheet).getByRole("button", { name: "Sign out" })).toBeTruthy();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(more);

    fireEvent.click(within(bar).getByRole("link", { name: "Traces" }));
    await waitFor(() => expect(window.location.pathname).toBe("/traces"));
    expect(new URLSearchParams(window.location.search).get("project")).toBe(alpha.id);
  });

  it("changes project and environment from the scope chip", async () => {
    stubServer();
    openOnPhone(`/overview?project=${alpha.id}`);
    fireEvent.click(await screen.findByRole("button", { name: /^Scope: Alpha/ }));
    const sheet = screen.getByRole("dialog", { name: "Project and environment" });
    expect(within(sheet).getByRole("button", { name: "Alpha" }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(await within(sheet).findByRole("button", { name: "staging" }));
    await waitFor(() => expect(new URLSearchParams(window.location.search).get("environment")).toBe("staging"));
    expect(screen.queryByRole("dialog")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /^Scope: Alpha/ }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Beta" }));
    await waitFor(() => expect(new URLSearchParams(window.location.search).get("project")).toBe(beta.id));
  });

  it("keeps log search inline and applies the identifiers from the filter sheet", async () => {
    const fetchMock = stubServer();
    openOnPhone(`/logs?project=${alpha.id}&service=api&range=15m`);
    fireEvent.change(await screen.findByRole("searchbox", { name: "Search log body" }), { target: { value: "timeout" } });
    fireEvent.click(screen.getByRole("button", { name: /^Filters/ }));
    const sheet = screen.getByRole("dialog", { name: "Filter logs" });
    expect((within(sheet).getByLabelText("Service") as HTMLInputElement).value).toBe("api");
    fireEvent.change(within(sheet).getByLabelText("Service"), { target: { value: "checkout" } });
    const traceID = "0123456789abcdef0123456789abcdef";
    fireEvent.change(within(sheet).getByLabelText("Trace ID"), { target: { value: traceID } });
    fireEvent.click(within(sheet).getByRole("button", { name: "Apply" }));
    await waitFor(() => {
      const params = new URLSearchParams(window.location.search);
      expect([params.get("service"), params.get("traceId"), params.get("search")]).toEqual(["checkout", traceID, "timeout"]);
    });
    await waitFor(() => expect(fetchMock.mock.calls.some(([path]) => String(path).includes("service=checkout") && String(path).includes("search=timeout"))).toBe(true));
  });
});
