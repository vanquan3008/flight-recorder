import type { ReactNode } from "react";
import { AlertTriangle, Check, ChevronDown, Copy, LoaderCircle, RefreshCw } from "lucide-react";
import { useState } from "react";
import { APIError } from "./api";

export const formatTime = (value?: string) =>
  value
    ? new Date(value).toLocaleString([], { dateStyle: "medium", timeStyle: "short" })
    : "—";

export const formatDuration = (nanoseconds?: number) => {
  if (!nanoseconds) return "—";
  if (nanoseconds < 1e6) return `${(nanoseconds / 1e3).toFixed(1)}µs`;
  if (nanoseconds < 1e9) return `${(nanoseconds / 1e6).toFixed(1)}ms`;
  return `${(nanoseconds / 1e9).toFixed(2)}s`;
};

export const json = (value: unknown) => {
  if (typeof value === "string") {
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return value;
    }
  }
  return JSON.stringify(value ?? {}, null, 2);
};

export function PageHeader({ title, children, action }: { title: string; children: ReactNode; action?: ReactNode }) {
  return <header className="page-header"><div><h1>{title}</h1><p>{children}</p></div>{action}</header>;
}

export function Panel({ title, meta, children, className = "" }: { title: string; meta?: ReactNode; children: ReactNode; className?: string }) {
  return <section className={`panel ${className}`}><header className="panel-head"><h2>{title}</h2>{meta && <span className="muted">{meta}</span>}</header>{children}</section>;
}

export function Badge({ value }: { value?: string }) {
  const label = value || "—";
  return <span className={`badge ${label.toUpperCase()}`}>{label}</span>;
}

export type TableRow = ReactNode[];

export function DataTable({ headers, rows, empty }: { headers: string[]; rows: TableRow[]; empty?: ReactNode }) {
  if (!rows.length) return <Empty>{empty || "No data yet."}</Empty>;
  return <div className="table-wrap" tabIndex={0} role="region" aria-label="Data table, scrollable"><table><thead><tr>{headers.map((header) => <th key={header}>{header}</th>)}</tr></thead><tbody>{rows.map((row, rowIndex) => <tr key={rowIndex}>{row.map((cell, cellIndex) => <td key={cellIndex}>{cell}</td>)}</tr>)}</tbody></table></div>;
}

export function Empty({ children }: { children: ReactNode }) {
  return <p className="empty">{children}</p>;
}

export function Loading() {
  return <div className="loading" role="status"><LoaderCircle aria-hidden="true" /> <span>Loading…</span></div>;
}

export function errorGuidance(error: unknown): { title: string; message: string } {
  const message = error instanceof Error ? error.message : "Unable to load this data.";
  if (error instanceof APIError && error.status === 401) return { title: "API key rejected", message: "This key is invalid, expired or revoked. Disconnect and connect with a current key." };
  if (error instanceof APIError && error.status === 403) return { title: "This key cannot open this view", message: `${message} Connect with a key that has the required scope.` };
  if (error instanceof TypeError) return { title: "Cannot reach TracePilot", message: "The server did not respond. Check the network connection and that the service is running, then retry." };
  return { title: "Unable to load this view", message };
}

export function ErrorState({ error, retry }: { error: unknown; retry?: () => void }) {
  const { title, message } = errorGuidance(error);
  const retryable = !(error instanceof APIError && (error.status === 401 || error.status === 403));
  return <div className="message error" role="alert"><AlertTriangle aria-hidden="true" /><div><strong>{title}</strong><p>{message}</p></div>{retry && retryable && <button onClick={retry}><RefreshCw size={15} /> Retry</button>}</div>;
}

/** Copies text and reports success or failure; clipboard access can be denied. */
export function CopyButton({ value, label }: { value: string; label: string }) {
  const [state, setState] = useState<"idle" | "copied" | "failed">("idle");
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setState("copied");
    } catch {
      setState("failed");
    }
    window.setTimeout(() => setState("idle"), 2000);
  };
  return <span className="copy-control">
    <button type="button" className="icon-button" aria-label={label} title={label} onClick={() => void copy()}>{state === "copied" ? <Check size={16} /> : <Copy size={16} />}</button>
    <span role="status" className={state === "failed" ? "form-error" : "sr-only"}>{state === "copied" ? "Copied" : state === "failed" ? "Clipboard access was denied. Select the text and copy it manually." : ""}</span>
  </span>;
}

/** Long values stay scannable: collapsed by default, expandable in place. */
export function ExpandableText({ value, limit = 360 }: { value: string; limit?: number }) {
  const [open, setOpen] = useState(false);
  if (value.length <= limit) return <pre className="inline-json">{value}</pre>;
  return <div className="expandable"><pre className="inline-json">{open ? value : `${value.slice(0, limit)}…`}</pre><button type="button" className="button-secondary compact" aria-expanded={open} onClick={() => setOpen(!open)}>{open ? "Show less" : `Show all (${value.length.toLocaleString()} chars)`}</button></div>;
}

export function LoadMore({ hasMore, loading, onClick }: { hasMore: boolean; loading: boolean; onClick: () => void }) {
  if (!hasMore) return null;
  return <div className="load-more"><button disabled={loading} onClick={onClick}>{loading ? "Loading…" : <><ChevronDown size={16} /> Load more</>}</button></div>;
}
