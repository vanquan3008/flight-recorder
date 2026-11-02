import type { ReactNode } from "react";
import { AlertTriangle, Check, ChevronDown, Copy, KeyRound, LoaderCircle, RefreshCw, WifiOff } from "lucide-react";
import { useState } from "react";
import { APIError } from "../api";

// ── Layout ─────────────────────────────────────────────────────────────────────

export function PageHeader({ title, children, action }: { title: string; children: ReactNode; action?: ReactNode }) {
  return (
    <header className="page-header flex flex-col md:flex-row md:items-start justify-between gap-4 mb-6">
      <div>
        <h1 className="text-2xl md:text-3xl font-black tracking-tight text-slate-900 dark:text-white mb-1.5">{title}</h1>
        <p className="text-[13.5px] text-slate-600 dark:text-slate-300 max-w-3xl m-0 leading-relaxed font-medium">{children}</p>
      </div>
      {action}
    </header>
  );
}

export function Panel({ title, meta, children, className = "" }: { title: string; meta?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={`panel rounded-[28px] backdrop-blur-3xl bg-white/75 dark:bg-slate-900/60 border border-white/80 dark:border-white/12 shadow-[inset_0_1px_1px_rgba(255,255,255,0.8),0_12px_40px_rgba(0,0,0,0.06)] dark:shadow-[inset_0_1px_1px_rgba(255,255,255,0.15),0_16px_48px_rgba(0,0,0,0.45)] overflow-hidden mb-6 ${className}`}>
      <header className="panel-head flex items-center justify-between min-h-[56px] px-6 py-3.5 border-b border-black/5 dark:border-white/10">
        <h2 className="text-[15px] font-bold text-slate-900 dark:text-white m-0 tracking-tight">{title}</h2>
        {meta && <span className="text-xs font-bold text-slate-600 dark:text-sky-300/80">{meta}</span>}
      </header>
      {children}
    </section>
  );
}

export function Badge({ value }: { value?: string }) {
  const label = value || "—";
  return <span className={`badge ${label.toUpperCase()}`}>{label}</span>;
}

export function Empty({ children }: { children: ReactNode }) {
  return <p className="empty p-8 text-center text-slate-600 dark:text-slate-400 font-medium text-sm m-0">{children}</p>;
}

// ── Table ──────────────────────────────────────────────────────────────────────

export type TableRow = ReactNode[];

export function DataTable({ headers, rows, empty }: { headers: string[]; rows: TableRow[]; empty?: ReactNode }) {
  if (!rows.length) return <Empty>{empty || "No data yet."}</Empty>;
  return (
    <div className="table-wrap overflow-auto" tabIndex={0} role="region" aria-label="Data table, scrollable">
      <table className="w-full min-w-[720px] border-collapse text-[13px]">
        <thead>
          <tr className="border-b border-black/5 dark:border-white/10">
            {headers.map((header) => (
              <th key={header} className="sticky top-0 z-10 backdrop-blur-md bg-white/95 dark:bg-slate-900/80 px-5 py-3.5 text-left text-[11px] font-black uppercase tracking-wider text-slate-700 dark:text-slate-300">
                {header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, rowIndex) => (
            <tr key={rowIndex} className="border-b border-black/5 dark:border-white/5 hover:bg-blue-500/5 dark:hover:bg-blue-400/5 transition-colors">
              {row.map((cell, cellIndex) => (
                <td key={cellIndex} className="px-5 py-3.5 align-middle text-slate-800 dark:text-slate-200 font-medium">{cell}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function LoadMore({ hasMore, loading, onClick }: { hasMore: boolean; loading: boolean; onClick: () => void }) {
  if (!hasMore) return null;
  return (
    <div className="load-more">
      <button disabled={loading} onClick={onClick}>
        {loading ? "Loading…" : <><ChevronDown size={16} /> Load more</>}
      </button>
    </div>
  );
}

// ── Loading skeleton ───────────────────────────────────────────────────────────

const skeletonWidths = [78, 64, 86, 52, 70];

function SkeletonRows() {
  return (
    <div className="skeleton-rows" aria-hidden="true">
      {skeletonWidths.map((width) => (
        <div className="sk-row" key={width}>
          <i className="sk sk-pill" />
          <i className="sk" style={{ width: `${width}%` }} />
          <i className="sk sk-figure" />
        </div>
      ))}
    </div>
  );
}

/**
 * Placeholder shapes while data loads. Inside a panel it is a few shimmering rows; as a whole page
 * (`page`) it sketches a header, a row of figures and a panel, so the layout does not jump when data arrives.
 */
export function Loading({ page = false }: { page?: boolean }) {
  const label = (
    <span className="loading-label">
      <LoaderCircle className="animate-spin" aria-hidden="true" size={14} />
      Loading…
    </span>
  );
  if (!page) return <div className="loading" role="status">{label}<SkeletonRows /></div>;
  return (
    <div className="loading loading-page" role="status">
      <div className="sk-head" aria-hidden="true">
        <i className="sk" style={{ width: 240, height: 24 }} />
        <i className="sk" style={{ width: 380, maxWidth: "70%", height: 12 }} />
      </div>
      <div className="sk-strip" aria-hidden="true">
        {[0, 1, 2, 3].map((index) => (
          <div key={index}>
            <i className="sk" style={{ width: "55%", height: 10 }} />
            <i className="sk" style={{ width: "32%", height: 24 }} />
            <i className="sk" style={{ width: "74%", height: 10 }} />
          </div>
        ))}
      </div>
      <div className="sk-panel">{label}<SkeletonRows /></div>
    </div>
  );
}

// ── Error notice ───────────────────────────────────────────────────────────────

export type ErrorGuidance = {
  title: string;
  message: string;
  kind: "auth" | "network" | "error";
  /** Technical context for IT; shown collapsed so staff are not asked to read it. */
  statusText?: string;
  detail?: string;
  hint?: string;
};

/** Plain-language guidance first; status codes, server messages and commands go in the details. */
export function errorGuidance(error: unknown): ErrorGuidance {
  const message = error instanceof Error ? error.message : "Unable to load this data.";
  if (error instanceof APIError) {
    if (error.status === 401) {
      return {
        kind: "auth",
        title: "Your access key is no longer valid",
        message: "It may have expired or been replaced. Disconnect, then connect again with a current key from your administrator.",
        statusText: "401 Unauthorized",
      };
    }
    if (error.status === 403) {
      return {
        kind: "auth",
        title: "You don't have access to this page",
        message: "Your key doesn't include permission for this view. Ask your administrator for a key that does.",
        statusText: "403 Forbidden",
        detail: message,
      };
    }
    if (error.status === 502) {
      return {
        kind: "network",
        title: "TracePilot isn't responding",
        message: "The service is not reachable right now. Wait a moment and try again. If it keeps happening, contact your administrator.",
        statusText: "502 Bad Gateway",
        hint: "go run ./backend/cmd/tracepilot serve",
      };
    }
    if (error.status === 503 || error.status === 504) {
      return {
        kind: "network",
        title: "TracePilot is busy",
        message: "The service is temporarily overloaded. Wait a few seconds and try again.",
        statusText: `HTTP ${error.status}`,
        hint: "go run ./backend/cmd/tracepilot",
      };
    }
    return { kind: "error", title: "We couldn't load this view", message, statusText: `HTTP ${error.status}` };
  }
  if (error instanceof TypeError) {
    return {
      kind: "network",
      title: "No connection to TracePilot",
      message: "Check your network connection and try again. If you're online, the service may be down; contact your administrator.",
      statusText: "Connection refused",
      hint: "go run ./backend/cmd/tracepilot serve",
    };
  }
  return { kind: "error", title: "We couldn't load this view", message };
}

const noticeIcons = { auth: KeyRound, network: WifiOff, error: AlertTriangle } as const;

/** One notification layout for every error: centred icon, short title, plain message, actions, then collapsed details. */
export function Notice({ guidance, variant = "inline", actions }: { guidance: ErrorGuidance; variant?: "inline" | "page" | "banner"; actions?: ReactNode }) {
  const Icon = noticeIcons[guidance.kind];
  const { statusText, detail, hint } = guidance;
  return (
    <div className={`notice notice-${variant} notice-${guidance.kind}`} role="alert">
      <span className="notice-icon" aria-hidden="true"><Icon size={variant === "banner" ? 18 : 22} /></span>
      <div className="notice-body">
        <strong className="notice-title">{guidance.title}</strong>
        <p className="notice-text">{guidance.message}</p>
        {(statusText || detail || hint) && (
          <details className="notice-details">
            <summary>Technical details</summary>
            {statusText && <p>{statusText}</p>}
            {detail && <p>{detail}</p>}
            {hint && <div className="notice-hint"><code>{hint}</code><CopyButton value={hint} label="Copy command" /></div>}
          </details>
        )}
      </div>
      {actions && <div className="notice-actions">{actions}</div>}
    </div>
  );
}

export function ErrorState({ error, retry, fullPage }: { error: unknown; retry?: () => void; fullPage?: boolean }) {
  const guidance = errorGuidance(error);
  const retryable = !(error instanceof APIError && (error.status === 401 || error.status === 403));
  const actions = (
    <>
      {retry && retryable && <button type="button" className="notice-btn" onClick={retry}><RefreshCw size={13} aria-hidden="true" /> Retry</button>}
      {fullPage && <a href="#/flow" className="notice-btn ghost">How it works</a>}
    </>
  );
  return <Notice guidance={guidance} variant={fullPage ? "page" : "inline"} actions={actions} />;
}

// ── Utility controls ───────────────────────────────────────────────────────────

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
  return (
    <span className="copy-control">
      <button type="button" className="icon-button" aria-label={label} title={label} onClick={() => void copy()}>
        {state === "copied" ? <Check size={16} /> : <Copy size={16} />}
      </button>
      <span role="status" className={state === "failed" ? "form-error" : "sr-only"}>
        {state === "copied" ? "Copied" : state === "failed" ? "Clipboard access was denied. Select the text and copy it manually." : ""}
      </span>
    </span>
  );
}

/** Long values stay scannable: collapsed by default, expandable in place. */
export function ExpandableText({ value, limit = 360 }: { value: string; limit?: number }) {
  const [open, setOpen] = useState(false);
  if (value.length <= limit) return <pre className="inline-json">{value}</pre>;
  return (
    <div className="expandable">
      <pre className="inline-json">{open ? value : `${value.slice(0, limit)}…`}</pre>
      <button type="button" className="button-secondary compact" aria-expanded={open} onClick={() => setOpen(!open)}>
        {open ? "Show less" : `Show all (${value.length.toLocaleString()} chars)`}
      </button>
    </div>
  );
}
