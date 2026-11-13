import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { useLiveStream } from "./live-stream";
import { setAccessToken, setProjectContext } from "./client";

function streamResponse(data = ""): Response {
  let sent = false;
  return { ok: true, status: 200, body: { getReader: () => ({ read: () => {
    if (data && !sent) { sent = true; return Promise.resolve({ done: false, value: new TextEncoder().encode(data) }); }
    return new Promise(() => {});
  } }) } } as unknown as Response;
}

describe("useLiveStream", () => {
  afterEach(() => {
    cleanup();
    setAccessToken("");
    setProjectContext({ project: "" });
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("refreshes an expired JWT and carries the project and environment on the stream", async () => {
    setAccessToken("expired-jwt");
    setProjectContext({ project: "alpha", environment: "production" });
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: { code: "TOKEN_EXPIRED" } }), { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ user: { id: "u1" }, memberships: [], access_token: "renewed-jwt" })))
      .mockResolvedValueOnce(streamResponse('data: {"type":"log","payload":{"event_id":"live-1"}}\r\n\r\n'));
    vi.stubGlobal("fetch", fetchMock);
    const onLog = vi.fn();
    const { result } = renderHook(() => useLiveStream({ enabled: true, credential: "session", onLog }));
    await waitFor(() => expect(onLog).toHaveBeenCalledWith({ event_id: "live-1" }));
    expect(result.current.connected).toBe(true);
    const headers = new Headers(fetchMock.mock.calls[2][1].headers);
    expect(headers.get("Authorization")).toBe("Bearer renewed-jwt");
    expect(headers.get("X-Project-ID")).toBe("alpha");
    expect(headers.get("X-Environment")).toBe("production");
  });

  it("aborts the old stream and connects in the new scope", async () => {
    setAccessToken("jwt");
    setProjectContext({ project: "alpha", environment: "staging" });
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(streamResponse()));
    vi.stubGlobal("fetch", fetchMock);
    const { rerender } = renderHook(() => useLiveStream({ enabled: true, credential: "session" }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    setProjectContext({ project: "beta", environment: "production" });
    rerender();
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(fetchMock.mock.calls[0][1].signal.aborted).toBe(true);
    expect(new Headers(fetchMock.mock.calls[1][1].headers).get("X-Project-ID")).toBe("beta");
  });

  it("reports a denied stream without retrying permanent access failures", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { message: "not allowed" } }), { status: 403 }));
    vi.stubGlobal("fetch", fetchMock);
    const { result } = renderHook(() => useLiveStream({ enabled: true, credential: "session" }));
    await waitFor(() => expect(result.current.error).toMatchObject({ status: 403 }));
    expect(result.current.connected).toBe(false);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("does not connect when disabled or credential is empty", () => {
    const { result } = renderHook(() =>
      useLiveStream({
        enabled: false,
        credential: "",
      }),
    );
    expect(result.current.connected).toBe(false);
  });

  it("connects and processes incoming SSE log events", async () => {
    const onLog = vi.fn();
    const encoder = new TextEncoder();
    const chunk = encoder.encode('data: {"type":"log","payload":{"event_id":"1","service_name":"order","body":"hello"}}\n\n');
    let readOnce = false;
    const mockReader = {
      read: vi.fn().mockImplementation(() => {
        if (!readOnce) {
          readOnce = true;
          return Promise.resolve({ done: false, value: chunk });
        }
        return new Promise(() => {});
      }),
    };
    const mockResponse = {
      ok: true,
      body: {
        getReader: () => mockReader,
      },
    };

    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(mockResponse as unknown as Response));

    const { result } = renderHook(() =>
      useLiveStream({
        enabled: true,
        credential: "test-token",
        onLog,
      }),
    );

    await waitFor(() => {
      expect(result.current.connected).toBe(true);
      expect(onLog).toHaveBeenCalledWith(
        expect.objectContaining({ event_id: "1", service_name: "order" }),
      );
    });
  });

  it("reconnects when the stream ends", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        body: { getReader: () => ({ read: () => Promise.resolve({ done: true }) }) },
      } as unknown as Response)
      .mockResolvedValue({
        ok: true,
        body: { getReader: () => ({ read: () => new Promise(() => {}) }) },
      } as unknown as Response);
    vi.stubGlobal("fetch", fetchMock);

    const { result, unmount } = renderHook(() =>
      useLiveStream({ enabled: true, credential: "test-token" }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2), { timeout: 2_000 });
    await waitFor(() => expect(result.current.connected).toBe(true));
    unmount();
  });
});
