import type { ReactNode } from "react";
import { AlertTriangle, ChevronDown, LoaderCircle, RefreshCw } from "lucide-react";

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

export function DataTable({ headers, rows, empty }: { headers: string[]; rows: ReactNode[][]; empty?: string }) {
  if (!rows.length) return <Empty>{empty || "No data yet."}</Empty>;
  return <div className="table-wrap"><table><thead><tr>{headers.map((header) => <th key={header}>{header}</th>)}</tr></thead><tbody>{rows.map((row, rowIndex) => <tr key={rowIndex}>{row.map((cell, cellIndex) => <td key={cellIndex}>{cell}</td>)}</tr>)}</tbody></table></div>;
}

export function Empty({ children }: { children: ReactNode }) {
  return <p className="empty">{children}</p>;
}

export function Loading() {
  return <div className="loading" role="status"><LoaderCircle aria-hidden="true" /> <span>Loading…</span></div>;
}

export function ErrorState({ error, retry }: { error: unknown; retry?: () => void }) {
  const message = error instanceof Error ? error.message : "Unable to load this data.";
  return <div className="message error" role="alert"><AlertTriangle aria-hidden="true" /><div><strong>Unable to load this view</strong><p>{message}</p></div>{retry && <button onClick={retry}><RefreshCw size={15} /> Retry</button>}</div>;
}

export function LoadMore({ hasMore, loading, onClick }: { hasMore: boolean; loading: boolean; onClick: () => void }) {
  if (!hasMore) return null;
  return <div className="load-more"><button disabled={loading} onClick={onClick}>{loading ? "Loading…" : <><ChevronDown size={16} /> Load more</>}</button></div>;
}
