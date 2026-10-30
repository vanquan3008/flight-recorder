import { ArrowLeft } from "lucide-react";
import type { ReactNode } from "react";
import { Link, useLocation, type To } from "react-router-dom";

type Origin = { from?: string };

const labels: [string, string][] = [
  ["/incidents/", "incident"], ["/incidents", "incidents"], ["/traces/", "trace"], ["/traces", "traces"],
  ["/logs", "logs"], ["/topology", "topology"], ["/overview", "overview"], ["/settings", "settings"],
];

function originLabel(from: string) {
  const path = from.split("?")[0];
  return labels.find(([prefix]) => (prefix.endsWith("/") ? path.startsWith(prefix) : path === prefix))?.[1] ?? "previous page";
}

/** Link that remembers the list URL (path, filters, time window) it was opened from. */
export function DetailLink({ to, className = "table-link", children }: { to: To; className?: string; children: ReactNode }) {
  const location = useLocation();
  const origin: Origin = { from: `${location.pathname}${location.search}` };
  return <Link className={className} to={to} state={origin}>{children}</Link>;
}

/** Returns to where the operator came from with filters intact, or to a stable list when opened directly. */
export function BackLink({ fallback, fallbackLabel }: { fallback: string; fallbackLabel: string }) {
  const location = useLocation();
  const from = (location.state as Origin | null)?.from;
  const target = from || fallback;
  return <Link className="back-link" to={target}><ArrowLeft size={16} /> Back to {from ? originLabel(from) : fallbackLabel}</Link>;
}
