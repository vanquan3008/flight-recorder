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
  if (val === null || val === undefined) return <span className="text-faint">null</span>;
  if (typeof val === "boolean") return <span className="text-violet-600 dark:text-violet-300">{String(val)}</span>;
  if (typeof val === "number") return <span className="text-sky-700 dark:text-sky-300">{val}</span>;
  return <span>{typeof val === "string" ? val : JSON.stringify(val)}</span>;
}

export function KeyValueTable({ data, placeholder }: { data?: Record<string, unknown>; placeholder?: string }) {
  const [filter, setFilter] = useState("");
  const entries = useMemo(() => (!data || typeof data !== "object" ? [] : Object.entries(data)), [data]);

  const filtered = useMemo(() => {
    if (!filter.trim()) return entries;
    const q = filter.trim().toLowerCase();
    return entries.filter(([k, v]) => k.toLowerCase().includes(q) || (typeof v === "object" ? JSON.stringify(v) : String(v)).toLowerCase().includes(q));
  }, [entries, filter]);

  if (entries.length === 0) return <p className="m-0 px-4 py-8 text-center text-xs text-muted">No properties recorded.</p>;

  return (
    <div className="min-w-0">
      {entries.length > 4 && (
        <div className="relative mx-3 my-3">
          <Search size={12} className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-faint" />
          <input className="border border-glass-field-edge bg-glass-field text-text focus:border-accent focus:ring-2 focus:ring-accent/15 h-8 w-full rounded-md py-1 pr-7 pl-8 text-[11px]" aria-label={placeholder || "Filter properties"} placeholder={placeholder || "Filter properties…"} value={filter} onChange={(e) => setFilter(e.target.value)} />
          {filter && <button type="button" className="absolute right-2 top-1/2 -translate-y-1/2 text-muted" aria-label="Clear property filter" onClick={() => setFilter("")}><X size={12} /></button>}
        </div>
      )}
      <dl className="m-0 divide-y divide-line-soft">{filtered.map(([k, v]) => <div key={k} className="grid grid-cols-[minmax(0,45%)_minmax(0,1fr)] items-start gap-3 px-4 py-2.5 text-[11px]">
        <dt className="break-words font-mono leading-relaxed text-muted">{k}</dt>
        <dd className="m-0 flex min-w-0 items-start justify-between gap-1"><span className="min-w-0 break-all font-mono leading-relaxed">{renderVal(v)}</span><span className="shrink-0 opacity-50 hover:opacity-100 focus-within:opacity-100"><CopyButton value={typeof v === "object" ? JSON.stringify(v) : String(v)} label={`Copy ${k}`} /></span></dd>
      </div>)}</dl>
      {filtered.length === 0 && <p className="m-0 px-4 py-6 text-center text-xs text-muted">No matching properties for “{filter}”.</p>}
    </div>
  );
}
