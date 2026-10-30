import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { App } from "../../app/app";

const respond = (data: unknown, status = 200) => new Response(JSON.stringify(data), { status });
const keyID = "01900000-0000-7000-8000-0000000000aa";
const existing = { id: keyID, name: "checkout-ingest", scopes: ["ingest"], created_at: "2026-10-01T00:00:00Z" };

afterEach(() => { cleanup(); sessionStorage.clear(); window.location.hash = ""; vi.unstubAllGlobals(); });

function open(hash: string, handler: (path: string, init?: RequestInit) => Response) {
  sessionStorage.setItem("tracepilot-api-key", "test-key");
  window.location.hash = hash;
  const fetch = vi.fn().mockImplementation((input: string, init?: RequestInit) => {
    const path = new URL(input, "http://localhost").pathname;
    return path === "/api/v1/traces" ? respond({ data: [] }) : handler(path, init);
  });
  vi.stubGlobal("fetch", fetch);
  render(<App />);
  return fetch;
}

it("tells a non-admin key why settings are unavailable instead of failing silently", async () => {
  open("#/settings", () => respond({ error: { message: "admin scope required" } }, 403));
  expect(await screen.findByText("You don't have access to this page")).toBeTruthy();
  expect(screen.getByText(/admin scope required/)).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
});

it("creates a key, shows its secret once and refreshes the list", async () => {
  let created = false;
  const fetch = open("#/settings", (path, init) => {
    if (path === "/api/v1/api-keys" && init?.method === "POST") {
      created = true;
      return respond({ api_key: { ...existing, id: "new", name: "billing" }, key: "tp_secret_value" }, 201);
    }
    return respond({ data: created ? [existing, { ...existing, id: "new", name: "billing" }] : [existing] });
  });
  fireEvent.change(await screen.findByLabelText("Name"), { target: { value: "billing" } });
  fireEvent.click(screen.getByRole("button", { name: "Create key" }));
  expect(await screen.findByText("tp_secret_value")).toBeTruthy();
  expect(screen.getByText(/shown only once/)).toBeTruthy();
  const post = fetch.mock.calls.find(([, init]) => init?.method === "POST")!;
  expect(JSON.parse(post[1].body)).toEqual({ name: "billing", scopes: ["ingest"] });
  expect(await screen.findByText("billing", { selector: "td" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "I have stored it" }));
  expect(screen.queryByText("tp_secret_value")).toBeNull();
});

it("requires confirmation before revoking a key and reports a failed mutation", async () => {
  const fetch = open("#/settings", (_path, init) => init?.method === "DELETE" ? respond({ error: { message: "key is already revoked" } }, 409) : respond({ data: [existing] }));
  fireEvent.click(await screen.findByRole("button", { name: "Revoke checkout-ingest" }));
  expect(fetch.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(false);
  fireEvent.click(screen.getByRole("button", { name: "Confirm revoke" }));
  expect(await screen.findByText("key is already revoked")).toBeTruthy();
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
