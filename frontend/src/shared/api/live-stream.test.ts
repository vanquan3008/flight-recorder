import { afterEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useLiveStream } from "./live-stream";

describe("useLiveStream", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
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
