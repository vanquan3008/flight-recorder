import { Search, X } from "lucide-react";
import type { ReactNode } from "react";
import { useMemo, useState } from "react";
import type { Span } from "../../shared/types";
import { CopyButton } from "../../shared/ui";

export interface ExceptionInfo {
  type: string;
  message: string;
  stacktrace?: string;
  timestamp?: string;
}

export function findException(span: Span): ExceptionInfo | null {
  const attrs = (span.attributes as Record<string, unknown>) || {};
  if (attrs["exception.type"] || attrs["exception.message"]) {
    return {
      type: String(attrs["exception.type"] || "Exception"),
      message: String(attrs["exception.message"] || "An unhandled exception occurred"),
      stacktrace: attrs["exception.stacktrace"] ? String(attrs["exception.stacktrace"]) : undefined,
    };
  }
  const events = span.events as Array<Record<string, unknown>> | undefined;
  if (Array.isArray(events)) {
    for (const ev of events) {
      if (!ev) continue;
      const name = String(ev.name || ev.Name || "");
      const evAttrs = (ev.attributes || ev.Attributes || {}) as Record<string, unknown>;
      if (name.toLowerCase() === "exception" || evAttrs["exception.type"] || evAttrs["exception.message"]) {
        return {
          type: String(evAttrs["exception.type"] || "Exception"),
          message: String(evAttrs["exception.message"] || "An unhandled exception occurred"),
          stacktrace: evAttrs["exception.stacktrace"] ? String(evAttrs["exception.stacktrace"]) : undefined,
          timestamp: ev.timestamp ? String(ev.timestamp) : (ev.Timestamp ? String(ev.Timestamp) : undefined),
        };
      }
    }
  }
  const rawSpan = span as Record<string, unknown>;
  if (rawSpan.status_message) {
    return { type: "SpanError", message: String(rawSpan.status_message) };
  }
  return null;
}

export function renderVal(val: unknown): ReactNode {
  if (val === null || val === undefined) return <span className="val-nil">null</span>;
  if (typeof val === "boolean") return <span className={`val-bool ${val}`}>{String(val)}</span>;
  if (typeof val === "number") return <span className="val-num">{val}</span>;
  if (typeof val === "string") return <span className="val-str">{val}</span>;
  return <span className="val-obj">{JSON.stringify(val)}</span>;
}

export function KeyValueTable({ data, placeholder }: { data?: Record<string, unknown>; placeholder?: string }) {
  const [filter, setFilter] = useState("");
  const entries = useMemo(() => (!data || typeof data !== "object" ? [] : Object.entries(data)), [data]);

  const filtered = useMemo(() => {
    if (!filter.trim()) return entries;
    const q = filter.toLowerCase();
    return entries.filter(([k, v]) => k.toLowerCase().includes(q) || String(v).toLowerCase().includes(q));
  }, [entries, filter]);

  if (entries.length === 0) return <div className="kv-empty">No properties available.</div>;

  return (
    <div className="kv-wrap">
      {entries.length > 4 && (
        <div className="kv-search-bar">
          <Search size={13} className="kv-search-icon" />
          <input className="kv-search-input" placeholder={placeholder || "Filter properties..."} value={filter} onChange={(e) => setFilter(e.target.value)} />
          {filter && <button type="button" className="kv-search-clear" onClick={() => setFilter("")}><X size={12} /></button>}
        </div>
      )}
      <div className="kv-table-scroll">
        <table className="kv-table">
          <tbody>
            {filtered.map(([k, v]) => (
              <tr key={k}>
                <td className="kv-key"><code>{k}</code></td>
                <td className="kv-value">
                  <span className="kv-val-render">{renderVal(v)}</span>
                  <CopyButton value={typeof v === "object" ? JSON.stringify(v) : String(v)} label={`Copy ${k}`} />
                </td>
              </tr>
            ))}
            {filtered.length === 0 && (
              <tr><td colSpan={2} className="kv-empty">No matching properties for &ldquo;{filter}&rdquo;</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
