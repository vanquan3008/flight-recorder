import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { App } from "../../app/app";

const respond = (data: unknown, status = 200) => new Response(JSON.stringify(data), { status });
const existing = { id: "01900000-0000-7000-8000-0000000000aa", name: "checkout-ingest", created_at: "2026-10-01T00:00:00Z", expires_at: "2099-10-01T00:00:00Z" };
const project = { id: "01900000-0000-7000-8000-0000000000a1", slug: "alpha", name: "Alpha", created_at: "2026-01-01T00:00:00Z" };
const retention = { retention_days: null, default_age_seconds: 604800, max_days: 106751 };
const session = { user: { id: "u2", email: "user@example.com", display_name: "A User", platform_admin: false, must_change_password: false, created_at: "2026-01-01T00:00:00Z" }, access_token: "test-access-token", expires_at: "2099-01-01T00:00:00Z" };

afterEach(() => { cleanup(); sessionStorage.clear(); window.location.hash = ""; vi.unstubAllGlobals(); });

function open(hash: string, handler: (path: string, init?: RequestInit) => Response) {
  localStorage.setItem("tracepilot-signed-in", "1");
  window.location.hash = hash;
  const fetch = vi.fn().mockImplementation((input: string, init?: RequestInit) => {
    const path = new URL(input, "http://localhost").pathname;
    return path === "/api/v1/traces" ? respond({ data: [] }) : handler(path, init);
  });
  vi.stubGlobal("fetch", fetch);
  render(<App />);
  return fetch;
}

it("shows a plain user only their projects, with no way into owner settings", async () => {
  localStorage.setItem("tracepilot-signed-in", "1");
  window.location.hash = "#/settings?tab=tokens";
  vi.stubGlobal("fetch", vi.fn().mockImplementation((input: string) => {
    const path = new URL(input, "http://localhost").pathname;
    if (path === "/api/v1/auth/refresh") return respond({ ...session, memberships: [{ project, role: "user" }] });
    if (path === `/api/v1/projects/${project.id}`) return respond({ project, role: "user", environments: [] });
    if (path.endsWith("/retention")) return respond(retention);
    return respond({ data: [] });
  }));
  render(<App />);
  expect(await screen.findByRole("button", { name: "Projects" })).toBeTruthy();
  for (const owner of ["Ingest tokens", "Webhooks", "Members", "Audit", "Accounts"]) expect(screen.queryByRole("button", { name: owner })).toBeNull();
  expect(screen.getByRole("heading", { name: "Your projects" })).toBeTruthy();
  expect(await screen.findByText("System default (7 days)")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Save retention" })).toBeNull();
});

it("sets project retention, confirms a decrease and restores the default", async () => {
  const updates: Array<number | null> = [];
  open("#/settings?tab=projects", (path, init) => {
    if (path === "/api/v1/auth/refresh") return respond({ ...session, memberships: [{ project, role: "owner" }] });
    if (path.endsWith("/retention")) {
      if (init?.method === "PUT") updates.push(JSON.parse(String(init.body)).retention_days);
      return respond({ ...retention, retention_days: updates.at(-1) ?? null });
    }
    if (path === `/api/v1/projects/${project.id}`) return respond({ project, role: "owner", environments: [] });
    return respond({ data: [] });
  });
  fireEvent.change(await screen.findByLabelText("Retention policy"), { target: { value: "custom" } });
  fireEvent.change(screen.getByLabelText("Retention days"), { target: { value: "3" } });
  fireEvent.click(screen.getByRole("button", { name: "Save retention" }));
  expect(updates).toEqual([]);
  fireEvent.click(screen.getByRole("button", { name: "Confirm shorter retention" }));
  expect(await screen.findByText("Retention saved.")).toBeTruthy();
  expect(updates).toEqual([3]);
  fireEvent.change(screen.getByLabelText("Retention policy"), { target: { value: "system" } });
  fireEvent.click(screen.getByRole("button", { name: "Save retention" }));
  await waitFor(() => expect(updates).toEqual([3, null]));
});

it("creates an ingest token, shows its secret once and refreshes the list", async () => {
  let created = false;
  const fetch = open("#/settings?tab=tokens", (path, init) => {
    if (path.endsWith("/ingest-tokens") && init?.method === "POST") {
      created = true;
      return respond({ token: { ...existing, id: "new", name: "billing" }, jwt: "eyJ.secret.value" }, 201);
    }
    return respond({ data: created ? [existing, { ...existing, id: "new", name: "billing" }] : [existing] });
  });
  fireEvent.change(await screen.findByLabelText("Name"), { target: { value: "billing" } });
  fireEvent.click(screen.getByRole("button", { name: "Create token" }));
  expect(await screen.findByText("eyJ.secret.value")).toBeTruthy();
  expect(screen.getByText(/shown only once/)).toBeTruthy();
  expect(screen.getByText(/OTEL_EXPORTER_OTLP_HEADERS/)).toBeTruthy();
  const post = fetch.mock.calls.find(([path, init]) => String(path).endsWith("/ingest-tokens") && init?.method === "POST")!;
  expect(JSON.parse(post[1].body)).toEqual({ name: "billing" });
  expect(new Headers(post[1].headers).get("Authorization")).toBe("Bearer test-access-token");
  expect(await screen.findByText("billing", { selector: "td" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "I have stored it" }));
  expect(screen.queryByText("eyJ.secret.value")).toBeNull();
});

it("requires confirmation before revoking an ingest token and reports a failed mutation", async () => {
  const fetch = open("#/settings?tab=tokens", (_path, init) => init?.method === "DELETE" ? respond({ error: { message: "ingest token not found" } }, 404) : respond({ data: [existing] }));
  fireEvent.click(await screen.findByRole("button", { name: "Revoke checkout-ingest" }));
  expect(fetch.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(false);
  fireEvent.click(screen.getByRole("button", { name: "Confirm revoke" }));
  expect(await screen.findByText("ingest token not found")).toBeTruthy();
  expect(fetch.mock.calls.filter(([, init]) => init?.method === "DELETE")).toHaveLength(1);
});

it("explains when webhook notifications are not enabled", async () => {
  open("#/settings?tab=webhooks", () => respond({ error: { message: "webhook notifications are not enabled" } }, 501));
  expect(await screen.findByText(/not enabled on this server/)).toBeTruthy();
});

it("lists webhook delivery history on demand", async () => {
  const hook = { id: "01900000-0000-7000-8000-0000000000bb", name: "on-call", url: "https://hooks.example.com/x", events: ["incident.opened"], enabled: true, created_at: "2026-10-01T00:00:00Z", updated_at: "2026-10-01T00:00:00Z" };
  open("#/settings?tab=webhooks", (path) => path.endsWith("/deliveries")
    ? respond({ data: [{ id: "d1", webhook_id: hook.id, event_type: "incident.opened", status: "FAILED", attempt: 2, max_attempts: 5, http_status: 502, last_error: "bad gateway", next_attempt_at: "2026-10-01T00:05:00Z", created_at: "2026-10-01T00:00:00Z" }] })
    : respond({ data: [hook] }));
  fireEvent.click(await screen.findByRole("button", { name: "Deliveries" }));
  expect(await screen.findByText("bad gateway")).toBeTruthy();
  await waitFor(() => expect(screen.getByText("2/5")).toBeTruthy());
});

it("lets a user leave a project after confirming", async () => {
  localStorage.setItem("tracepilot-signed-in", "1");
  window.location.hash = "#/settings";
  const fetch = vi.fn().mockImplementation((input: string, init?: RequestInit) => {
    const path = new URL(input, "http://localhost").pathname;
    if (path === "/api/v1/auth/refresh") return respond({ ...session, memberships: [{ project, role: "user" }] });
    if (init?.method === "DELETE") return new Response(null, { status: 204 });
    if (path === "/api/v1/auth/me") return respond({ user: session.user, memberships: [] });
    if (path === `/api/v1/projects/${project.id}`) return respond({ project, role: "user", environments: [] });
    if (path.endsWith("/retention")) return respond(retention);
    return respond({ data: [] });
  });
  vi.stubGlobal("fetch", fetch);
  render(<App />);
  fireEvent.click(await screen.findByRole("button", { name: "Leave Alpha" }));
  expect(fetch.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(false);
  fireEvent.click(screen.getByRole("button", { name: "Confirm leave" }));
  await waitFor(() => expect(fetch.mock.calls.some(([path, init]) => String(path) === `/api/v1/projects/${project.id}/members/u2` && init?.method === "DELETE")).toBe(true));
});
