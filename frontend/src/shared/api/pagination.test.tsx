import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { usePagedQuery } from "./client";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

it("bounds an explorer to 20 pages and tells the operator when more data exists", async () => {
  let page = 0;
  const fetchMock = vi.fn(() => Promise.resolve(new Response(JSON.stringify({ data: [{ id: ++page }], next_cursor: String(page) }))));
  vi.stubGlobal("fetch", fetchMock);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  const { result } = renderHook(() => usePagedQuery(["logs"], "/api/v1/logs", "session"), { wrapper });
  await waitFor(() => expect(result.current.isSuccess).toBe(true));
  for (let i = 1; i < 20; i++) {
    await act(async () => { await result.current.fetchNextPage(); });
    await waitFor(() => expect(result.current.data?.pages).toHaveLength(i + 1));
  }
  expect(result.current.data?.pages).toHaveLength(20);
  expect(result.current.hasNextPage).toBe(false);
  expect(result.current.limitReached).toBe(true);
  await act(async () => { await result.current.fetchNextPage(); });
  expect(fetchMock).toHaveBeenCalledTimes(20);
  client.clear();
});
