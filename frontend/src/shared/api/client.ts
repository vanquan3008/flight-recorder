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

function buildRequest(path: string, init: RequestInit, signedIn: boolean): RequestInit {
  const headers = new Headers(init.headers);
  if (signedIn && requestContext.token) {
    headers.set("Authorization", `Bearer ${requestContext.token}`);
    if (requestContext.project) headers.set("X-Project-ID", requestContext.project);
    if (requestContext.environment) headers.set("X-Environment", requestContext.environment);
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

/** Trades the refresh cookie for a new access token. Concurrent callers share one request. */
export function refreshAccess(): Promise<SessionView | null> {
  refreshing ??= (async () => {
    try {
      const response = await fetch(`${apiBase}${authPrefix}refresh`, buildRequest(`${authPrefix}refresh`, { method: "POST" }, false));
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

export async function api<T>(
  path: string,
  credential: string,
  init: RequestInit = {},
): Promise<T> {
  let response = await fetch(apiBase + path, buildRequest(path, init, Boolean(credential)));
  // An access token lasts minutes. When the server says it ran out, refresh once and repeat the call.
  if (response.status === 401 && credential && path !== `${authPrefix}refresh`) {
    const expired = (await response.clone().json().catch(() => undefined))?.error?.code === "TOKEN_EXPIRED";
    if (expired && (await refreshAccess())) response = await fetch(apiBase + path, buildRequest(path, init, true));
  }
  if (response.status === 204) return undefined as T;
  if (!response.ok) throw await failure(response);
  return response.json() as Promise<T>;
}

function pageURL(path: string, cursor: string): string {
  if (!cursor) return path;
  const url = new URL(path, window.location.origin);
  url.searchParams.set("cursor", cursor);
  return `${url.pathname}${url.search}`;
}

export function usePagedQuery<T>(key: QueryKey, path: string, credential: string) {
  return useInfiniteQuery({
    queryKey: [...key, path],
    enabled: Boolean(credential),
    initialPageParam: "",
    queryFn: ({ pageParam, signal }) =>
      api<Page<T>>(pageURL(path, pageParam), credential, { signal }),
    getNextPageParam: (lastPage) => lastPage.next_cursor || undefined,
  });
}

export function pageItems<T>(pages: Page<T>[] | undefined) {
  return pages?.flatMap((page) => page.data) ?? [];
}
