import { afterEach, describe, expect, it, vi } from "vitest";
import { APIError, api, queryString, sessionCredential, setAccessToken, setProjectContext } from "./index";
import { compactSearchParams } from "./params";

afterEach(() => vi.unstubAllGlobals());

describe("api", () => {
  it("only adds the JSON content type for a body", async () => {
    const fetchMock = vi.fn().mockImplementation(() => new Response(JSON.stringify({ ok: true }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    await expect(api<{ ok: boolean }>("/api/v1/auth/status", "")).resolves.toEqual({ ok: true });
    await api("/api/v1/auth/login", "", { method: "POST", body: "{}" });
    const [[, plain], [, withBody]] = fetchMock.mock.calls as [string, RequestInit][];
    expect(new Headers(plain.headers).get("Content-Type")).toBeNull();
    expect(new Headers(plain.headers).get("Authorization")).toBeNull();
    expect(new Headers(withBody.headers).get("Content-Type")).toBe("application/json");
  });

  it("sends a signed-in request with the bearer token and the project instead of a key", async () => {
    const fetchMock = vi.fn().mockImplementation(() => new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    setAccessToken("jwt-1");
    setProjectContext({ project: "project-1", environment: "staging" });
    await api("/api/v1/traces", sessionCredential);
    const [[, read]] = fetchMock.mock.calls as [string, RequestInit][];
    const headers = new Headers(read.headers);
    expect(headers.get("Authorization")).toBe("Bearer jwt-1");
    expect(headers.get("X-Project-ID")).toBe("project-1");
    expect(headers.get("X-Environment")).toBe("staging");
    expect(headers.get("X-API-Key")).toBeNull();
    expect(read.credentials).toBeUndefined();
    setAccessToken("");
    setProjectContext({ project: "", environment: "" });
  });

  it("sends the refresh cookie only to the auth routes, with the header a cross-site page cannot add", async () => {
    const fetchMock = vi.fn().mockImplementation(() => new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    setAccessToken("jwt-1");
    await api("/api/v1/auth/me", sessionCredential);
    await api("/api/v1/traces", sessionCredential);
    const [[, auth], [, other]] = fetchMock.mock.calls as [string, RequestInit][];
    expect(auth.credentials).toBe("include");
    expect(new Headers(auth.headers).get("X-TracePilot-Refresh")).toBe("1");
    expect(other.credentials).toBeUndefined();
    expect(new Headers(other.headers).get("X-TracePilot-Refresh")).toBeNull();
    setAccessToken("");
  });

  it("refreshes an expired access token once and repeats the call", async () => {
    const view = { user: { id: "u1" }, memberships: [], access_token: "jwt-2", expires_at: "2099-01-01T00:00:00Z" };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: { code: "TOKEN_EXPIRED", message: "authentication required" } }), { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(view), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: [] }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    setAccessToken("jwt-old");
    await expect(api("/api/v1/traces", sessionCredential)).resolves.toEqual({ data: [] });
    const calls = fetchMock.mock.calls as [string, RequestInit][];
    expect(calls[1][0]).toContain("/api/v1/auth/refresh");
    expect(new Headers(calls[2][1].headers).get("Authorization")).toBe("Bearer jwt-2");
    setAccessToken("");
  });

  it("does not refresh for any other 401", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { code: "UNAUTHORIZED", message: "authentication required" } }), { status: 401 }));
    vi.stubGlobal("fetch", fetchMock);
    setAccessToken("jwt-old");
    await expect(api("/api/v1/traces", sessionCredential)).rejects.toMatchObject({ status: 401 });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    setAccessToken("");
  });

  it("keeps the original request scope if the project changes during refresh", async () => {
    setAccessToken("jwt-old");
    setProjectContext({ project: "alpha", environment: "staging" });
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: { code: "TOKEN_EXPIRED" } }), { status: 401 }))
      .mockImplementationOnce(() => {
        setProjectContext({ project: "beta", environment: "production" });
        return Promise.resolve(new Response(JSON.stringify({ user: { id: "u1" }, memberships: [], access_token: "jwt-new" })));
      })
      .mockResolvedValueOnce(new Response("{}"));
    vi.stubGlobal("fetch", fetchMock);
    await api("/api/v1/traces", sessionCredential);
    const headers = new Headers(fetchMock.mock.calls[2][1].headers);
    expect(headers.get("Authorization")).toBe("Bearer jwt-new");
    expect(headers.get("X-Project-ID")).toBe("alpha");
    expect(headers.get("X-Environment")).toBe("staging");
    setAccessToken("");
    setProjectContext({ project: "" });
  });

  it("exposes API errors without leaking the response body", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { code: "FORBIDDEN", message: "scope required" } }), { status: 403 })));
    await expect(api("/api/v1/rules", "key")).rejects.toMatchObject<Partial<APIError>>({ status: 403, code: "FORBIDDEN", message: "scope required" });
  });
});

describe("queryString", () => {
  it("keeps the page limit and omits blank filters", () => {
    expect(queryString({ service: " checkout ", status: "" })).toBe("limit=50&service=checkout");
  });
});

describe("compactSearchParams", () => {
  it("trims populated fields and omits blank fields", () => {
    const form = document.createElement("form");
    form.innerHTML = '<input name="service" value=" checkout "><input name="status" value=" ">';
    expect(compactSearchParams(form).toString()).toBe("service=checkout");
  });
});
