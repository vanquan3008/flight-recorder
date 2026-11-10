import { ArrowLeft } from "lucide-react";
import type { ReactNode } from "react";
import { Link, useLocation, type To } from "react-router-dom";

type Origin = { from?: string };

/** Carries scope on shared links; section navigation also keeps the selected window. */
export function contextURL(to: string, search: string, includeTime = false): string {
  if (!to.startsWith("/")) return to;
  const target = new URL(to, window.location.origin);
  const current = new URLSearchParams(search);
  for (const key of includeTime ? ["project", "environment", "range", "from", "to"] : ["project", "environment"]) {
    if (!target.searchParams.has(key) && current.has(key)) target.searchParams.set(key, current.get(key)!);
  }
  return `${target.pathname}${target.search}${target.hash}`;
}

const labels: [string, string][] = [
  ["/incidents/", "incident"], ["/incidents", "incidents"], ["/traces/", "trace"], ["/traces", "traces"],
  ["/logs", "logs"], ["/topology", "topology"], ["/overview", "overview"], ["/settings", "settings"],
];

function originLabel(from: string) {
  const path = from.split("?")[0];
  return labels.find(([prefix]) => (prefix.endsWith("/") ? path.startsWith(prefix) : path === prefix))?.[1] ?? "previous page";
}

/** Link that remembers the list URL (path, filters, time window) it was opened from. */
export function DetailLink({ to, className = "table-link font-medium text-accent no-underline hover:underline", title, children, ...rest }: { to: To; className?: string; title?: string; children: ReactNode; [key: string]: unknown }) {
  const location = useLocation();
  const origin: Origin = { from: `${location.pathname}${location.search}` };
  const target = typeof to === "string" ? contextURL(to, location.search) : to;
  return <Link className={className} to={target} state={origin} title={title} {...rest}>{children}</Link>;
}

/** Returns to where the operator came from with filters intact, or to a stable list when opened directly. */
export function BackLink({ fallback, fallbackLabel }: { fallback: string; fallbackLabel: string }) {
  const location = useLocation();
  const from = (location.state as Origin | null)?.from;
  const target = from || fallback;
  return <Link className="back-link inline-flex items-center gap-1.5 text-xs text-muted hover:text-text" to={target}><ArrowLeft size={16} /> Back to {from ? originLabel(from) : fallbackLabel}</Link>;
}
