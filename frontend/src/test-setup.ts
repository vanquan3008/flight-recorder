import { configure } from "@testing-library/react";
import { afterEach, vi } from "vitest";

// The console resumes its session before it renders; under a parallel test run that can outlast the default second.
configure({ asyncUtilTimeout: 4000 });

// Most tests open the console as a signed-in owner. The console resumes a session by asking the server to
// refresh, so wrap each stubbed `fetch` to answer that with a canned session unless the test answers it itself.
const session = {
  user: { id: "user-1", email: "owner@example.com", display_name: "Test Owner", platform_admin: false, must_change_password: false, created_at: "2026-01-01T00:00:00Z" },
  memberships: [{ project: { id: "01900000-0000-7000-8000-0000000000a1", slug: "alpha", name: "Alpha", created_at: "2026-01-01T00:00:00Z" }, role: "owner" }],
  access_token: "test-access-token",
  expires_at: "2099-01-01T00:00:00Z",
};

type FetchLike = (input: string, init?: RequestInit) => Response | Promise<Response>;

const stubGlobal = vi.stubGlobal.bind(vi);
vi.stubGlobal = ((name: string, value: unknown) => {
  if (name !== "fetch" || typeof value !== "function") return stubGlobal(name, value);
  const inner = value as FetchLike;
  const wrapped = vi.fn<FetchLike>(async (input, init) => {
    const response = await inner(input, init);
    const path = new URL(String(input), "http://localhost").pathname;
    if (path !== "/api/v1/auth/refresh") return response;
    const body = await response.clone().json().catch(() => undefined);
    return body?.user ? response : new Response(JSON.stringify(session), { status: 200 });
  });
  return stubGlobal(name, wrapped);
}) as typeof vi.stubGlobal;

afterEach(() => localStorage.clear());
