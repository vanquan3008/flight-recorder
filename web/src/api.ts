import { useInfiniteQuery } from "@tanstack/react-query";
import type { QueryKey } from "@tanstack/react-query";
import type { Page } from "./types";

export class APIError extends Error {
  constructor(
    readonly status: number,
    message: string,
    readonly code?: string,
  ) {
    super(message);
  }
}

export async function api<T>(
  path: string,
  apiKey: string,
  init: RequestInit = {},
): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("X-API-Key", apiKey);
  if (init.body) headers.set("Content-Type", "application/json");
  const response = await fetch(path, { ...init, headers });
  if (response.status === 204) return undefined as T;
  if (!response.ok) {
    let body: { error?: { code?: string; message?: string } } | undefined;
    try {
      body = await response.json();
    } catch {
      // Status text is a useful fallback for a proxy or non-JSON response.
    }
    throw new APIError(
      response.status,
      body?.error?.message || `Request failed (${response.status})`,
      body?.error?.code,
    );
  }
  return response.json() as Promise<T>;
}

function pageURL(path: string, cursor: string): string {
  if (!cursor) return path;
  const url = new URL(path, window.location.origin);
  url.searchParams.set("cursor", cursor);
  return `${url.pathname}${url.search}`;
}

export function usePagedQuery<T>(key: QueryKey, path: string, apiKey: string) {
  return useInfiniteQuery({
    queryKey: [...key, path],
    enabled: Boolean(apiKey),
    initialPageParam: "",
    queryFn: ({ pageParam, signal }) =>
      api<Page<T>>(pageURL(path, pageParam), apiKey, { signal }),
    getNextPageParam: (lastPage) => lastPage.next_cursor || undefined,
  });
}

export const pageItems = <T,>(pages: Page<T>[] | undefined) =>
  pages?.flatMap((page) => page.data) ?? [];

export function queryString(values: Record<string, string | undefined>): string {
  const query = new URLSearchParams({ limit: "50" });
  Object.entries(values).forEach(([key, value]) => {
    if (value?.trim()) query.set(key, value.trim());
  });
  return query.toString();
}
