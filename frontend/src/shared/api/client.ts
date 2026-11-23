import { useInfiniteQuery } from "@tanstack/react-query";
import type { QueryKey } from "@tanstack/react-query";
import type { SessionView } from "../session";
import type { Page } from "../types";

// Where the backend lives. Empty means the same origin, which is how the Vite dev
// and preview servers reach it through their proxy. Set VITE_API_BASE_URL when the
// console is hosted separately from the API (the backend must then list that origin
// in TRACEPILOT_HTTP_ALLOWED_ORIGINS).
const apiBase = (import.meta.env.VITE_API_BASE_URL ?? "").replace(/\/$/, "");

export class APIError extends Error {
  constructor(
    readonly status: number,
    message: string,
    readonly code?: string,
  ) {
    super(message);
  }
}

// Calls take the credential of whoever is signed in. It is only a marker (empty means nobody): the
// access token itself lives in this module, in memory, so it is never written to storage.
export const sessionCredential = "session";

type RequestContext = { token: string; project: string; environment: string };
let requestContext: RequestContext = { token: "", project: "", environment: "" };
let refreshListener: ((view: SessionView) => void) | undefined;

/** Sets the access token a signed-in request carries. */
export function setAccessToken(token: string) {
  requestContext = { ...requestContext, token };
}

/** Sets the project (and environment) signed-in requests act in. Environments belong to one project, so an omitted one means the project's default. */
export function setProjectContext(next: { project: string; environment?: string }) {
  requestContext = { ...requestContext, project: next.project, environment: next.environment ?? "" };
}

/** Called whenever a refresh produced a new session, so the console can keep its own state in step. */
export function onSessionRefreshed(listener: ((view: SessionView) => void) | undefined) {
  refreshListener = listener;
}

const authPrefix = "/api/v1/auth/";

function buildRequest(path: string, init: RequestInit, signedIn: boolean, context = requestContext): RequestInit {
  const headers = new Headers(init.headers);
  if (signedIn && context.token) {
    headers.set("Authorization", `Bearer ${context.token}`);
    if (context.project) headers.set("X-Project-ID", context.project);
    if (context.environment) headers.set("X-Environment", context.environment);
  }
  if (init.body) headers.set("Content-Type", "application/json");
  const options: RequestInit = { ...init, headers };
  if (path.startsWith(authPrefix)) {
    // The refresh cookie travels only to the auth routes, and only with a header a cross-site page cannot add.
    headers.set("X-TracePilot-Refresh", "1");
    options.credentials = "include";
  }
  return options;
}

let refreshing: Promise<SessionView | null> | undefined;

// Every tab shares one refresh cookie and each refresh replaces it. Tabs take turns under this lock,
// so a tab waiting its turn (e.g. all of them after the computer wakes) sends the cookie the previous
// refresh set instead of the one that refresh just retired.
const refreshLock = "tracepilot-refresh";

function requestRefresh(): Promise<Response> {
  return fetch(`${apiBase}${authPrefix}refresh`, buildRequest(`${authPrefix}refresh`, { method: "POST" }, false));
}

/** Trades the refresh cookie for a new access token. Callers in this tab share one request; tabs queue. */
export function refreshAccess(): Promise<SessionView | null> {
  refreshing ??= (async () => {
    try {
      const locks = typeof navigator === "undefined" ? undefined : navigator.locks;
      const response = locks ? await locks.request(refreshLock, requestRefresh) : await requestRefresh();
      if (!response.ok) return null;
      const view = (await response.json()) as SessionView;
      if (view.access_token) setAccessToken(view.access_token);
      refreshListener?.(view);
      return view;
    } catch {
      return null;
    } finally {
      refreshing = undefined;
    }
  })();
  return refreshing;
}

async function failure(response: Response): Promise<APIError> {
  let body: { error?: { code?: string; message?: string } } | undefined;
  try {
    body = await response.json();
  } catch {
    // Status text is a useful fallback for a proxy or non-JSON response.
  }
  return new APIError(response.status, body?.error?.message || `Request failed (${response.status})`, body?.error?.code);
}

/** Includes scope and token refresh for JSON requests and streaming responses. */
export async function apiResponse(
  path: string,
  credential: string,
  init: RequestInit = {},
): Promise<Response> {
  const context = requestContext;
  let response = await fetch(apiBase + path, buildRequest(path, init, Boolean(credential), context));
  // An access token lasts minutes. When the server says it ran out, refresh once and repeat the call.
  if (response.status === 401 && credential && path !== `${authPrefix}refresh`) {
    const expired = (await response.clone().json().catch(() => undefined))?.error?.code === "TOKEN_EXPIRED";
    if (expired && (await refreshAccess())) {
      init.signal?.throwIfAborted();
      response = await fetch(apiBase + path, buildRequest(path, init, true, { ...context, token: requestContext.token }));
    }
  }
  if (!response.ok) throw await failure(response);
  return response;
}

export async function api<T>(path: string, credential: string, init: RequestInit = {}): Promise<T> {
  const response = await apiResponse(path, credential, init);
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export function requestScopeKey(): string {
  return `${requestContext.project}:${requestContext.environment}`;
}

function pageURL(path: string, cursor: string): string {
  if (!cursor) return path;
  const url = new URL(path, window.location.origin);
  url.searchParams.set("cursor", cursor);
  return `${url.pathname}${url.search}`;
}

export function usePagedQuery<T>(key: QueryKey, path: string, credential: string, enabled = true) {
  const query = useInfiniteQuery({
    queryKey: [...key, requestScopeKey(), path],
    enabled: Boolean(credential) && enabled,
    initialPageParam: "",
    queryFn: ({ pageParam, signal }) =>
      api<Page<T>>(pageURL(path, pageParam), credential, { signal }),
    getNextPageParam: (lastPage, pages) => pages.length < 20 ? lastPage.next_cursor || undefined : undefined,
  });
  return { ...query, limitReached: Boolean(query.data?.pages.length === 20 && query.data.pages.at(-1)?.next_cursor) };
}

export function pageItems<T>(pages: Page<T>[] | undefined) {
  return pages?.flatMap((page) => page.data) ?? [];
}
