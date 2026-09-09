import { afterEach, describe, expect, it, vi } from "vitest";
import { APIError, api, queryString } from "./api";

afterEach(() => vi.unstubAllGlobals());

describe("api", () => {
  it("sends the API key and only adds JSON content type for a body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    await expect(api<{ ok: boolean }>("/api/v1/traces", "key-123")).resolves.toEqual({ ok: true });
    const [, options] = fetchMock.mock.calls[0] as [string, RequestInit];
    const headers = new Headers(options.headers);
    expect(headers.get("X-API-Key")).toBe("key-123");
    expect(headers.get("Content-Type")).toBeNull();
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
