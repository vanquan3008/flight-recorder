import { secondaryButtonClass } from "./styles";
import type { ReactNode } from "react";
import { AlertTriangle, Check, Copy, KeyRound, LoaderCircle, RefreshCw, WifiOff } from "lucide-react";
import { useState } from "react";
import { APIError } from "../api";
import { formatTime, formatTimeIso } from "./formatters";

// ── Layout ─────────────────────────────────────────────────────────────────────

export function PageHeader({ title, children, action }: { title: string; children: ReactNode; action?: ReactNode }) {
  return (
    <header className="page-header mb-3.5 flex flex-col justify-between gap-2 min-[901px]:flex-row min-[901px]:items-start">
      <div className="min-w-0">
        <h1 className="mb-0.5 text-[22px] leading-tight font-semibold tracking-tight text-text">{title}</h1>
        <p className="m-0 max-w-3xl text-[13px] leading-relaxed text-muted">{children}</p>
      </div>
      {action}
    </header>
  );
}

export function Panel({ title, meta, children, className = "", fit = false }: { title: ReactNode; meta?: ReactNode; children: ReactNode; className?: string; fit?: boolean }) {
  return (
    <section className={`panel overflow-hidden rounded-3xl border border-glass-edge bg-glass backdrop-glass shadow-glass ${fit ? "panel-fit flex min-h-40 flex-1 flex-col [&_.table-wrap]:min-h-0 [&_.table-wrap]:flex-1 [&_.empty]:flex-1" : "mb-3.5"} ${className}`}>
      <header className="panel-head flex min-h-10 shrink-0 items-center justify-between gap-3 border-b border-line-soft px-4 py-1.5">
        <h2 className="m-0 text-[13px] font-semibold text-text">{title}</h2>
        {meta && <span className="text-[11.5px] font-medium text-muted">{meta}</span>}
      </header>
      {children}
    </section>
  );
}

export function Badge({ value }: { value?: string }) {
  const label = value || "—";
  const status = label.toUpperCase();
  const tone = ["ERROR", "HIGH", "CRITICAL", "FATAL", "OPEN", "FAILED"].includes(status)
    ? "border-rose-500/25 bg-danger-bg text-danger-fg"
    : ["MEDIUM", "WARN", "WARNING", "INVESTIGATING", "PENDING"].includes(status)
    ? "border-amber-500/25 bg-warn-bg text-warn-fg"
    : ["LOW", "INFO", "INFORMATION", "OK", "RESOLVED", "DELIVERED", "ACTIVE"].includes(status)
    ? "border-emerald-500/25 bg-ok-bg text-ok-fg" : "border-transparent bg-neutral-bg text-neutral-fg";
  return <span className={`badge ${status} inline-flex items-center gap-1.5 rounded-full border px-2 py-px text-[10px] font-bold tracking-wide whitespace-nowrap uppercase ${tone}`}><i className="size-1.25 rounded-full bg-current opacity-90" aria-hidden="true" />{label}</span>;
}

/** Clean single-line technical ISO timestamp (YYYY-MM-DD HH:mm:ss.SSS) */
export function TimeCell({ value, className = "" }: { value?: string; className?: string }) {
  const text = formatTimeIso(value);
  if (text === "—") return <span>—</span>;
  return (
    <span className={`time-cell-flat inline-block whitespace-nowrap font-mono text-xs tracking-tight text-muted tabular-nums ${className}`.trim()} title={formatTime(value)}>
      {text}
    </span>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return <p className="empty m-0 p-8 text-center text-sm text-muted flex items-center justify-center font-medium ">{children}</p>;
}

// ── Table ──────────────────────────────────────────────────────────────────────

const skeletonWidths = [78, 64, 86, 52, 70];

function SkeletonRows() {
  return (
    <div className="skeleton-rows grid" aria-hidden="true">
      {skeletonWidths.map((width) => (
        <div className="sk-row grid grid-cols-[64px_minmax(0,1fr)_72px] items-center gap-5 border-t border-glass-divider py-3 max-[620px]:grid-cols-[48px_minmax(0,1fr)]" key={width}>
          <i className="sk block h-3 rounded-md bg-linear-to-r from-text/7 via-text/14 to-text/7 bg-size-[240%_100%] motion-safe:animate-skeleton sk-pill h-4.5 rounded-full" />
          <i className="sk block h-3 rounded-md bg-linear-to-r from-text/7 via-text/14 to-text/7 bg-size-[240%_100%] motion-safe:animate-skeleton" style={{ width: `${width}%` }} />
          <i className="sk block h-3 rounded-md bg-linear-to-r from-text/7 via-text/14 to-text/7 bg-size-[240%_100%] motion-safe:animate-skeleton sk-figure w-14 justify-self-end max-[620px]:hidden" />
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
    <span className="loading-label inline-flex items-center gap-2 text-xs font-semibold text-muted [&_svg]:text-accent">
      <LoaderCircle className="animate-spin" aria-hidden="true" size={14} />
      Loading…
    </span>
  );
  if (!page) return <div className="loading grid gap-3 px-4 py-4.5 motion-safe:animate-appear" role="status">{label}<SkeletonRows /></div>;
  return (
    <div className="loading grid grid-cols-[minmax(0,1fr)] gap-3 px-4 py-4.5 motion-safe:animate-appear loading-page gap-3.5 p-0" role="status">
      <div className="sk-head grid grid-cols-[minmax(0,1fr)] gap-2.5 pt-1 pb-1.5" aria-hidden="true">
        <i className="sk block h-3 rounded-md bg-linear-to-r from-text/7 via-text/14 to-text/7 bg-size-[240%_100%] motion-safe:animate-skeleton" style={{ width: 240, height: 24 }} />
        <i className="sk block h-3 rounded-md bg-linear-to-r from-text/7 via-text/14 to-text/7 bg-size-[240%_100%] motion-safe:animate-skeleton" style={{ width: 380, maxWidth: "70%", height: 12 }} />
      </div>
      <div className="sk-strip border border-glass-edge bg-glass backdrop-glass shadow-glass grid grid-cols-4 overflow-hidden rounded-[18px] max-[900px]:grid-cols-2 [&>div]:grid [&>div]:gap-2.5 [&>div]:border-r [&>div]:border-glass-divider [&>div]:p-4 [&>div:last-child]:border-r-0" aria-hidden="true">
        {[0, 1, 2, 3].map((index) => (
          <div key={index}>
            <i className="sk block h-3 rounded-md bg-linear-to-r from-text/7 via-text/14 to-text/7 bg-size-[240%_100%] motion-safe:animate-skeleton" style={{ width: "55%", height: 10 }} />
            <i className="sk block h-3 rounded-md bg-linear-to-r from-text/7 via-text/14 to-text/7 bg-size-[240%_100%] motion-safe:animate-skeleton" style={{ width: "32%", height: 24 }} />
            <i className="sk block h-3 rounded-md bg-linear-to-r from-text/7 via-text/14 to-text/7 bg-size-[240%_100%] motion-safe:animate-skeleton" style={{ width: "74%", height: 10 }} />
          </div>
        ))}
      </div>
      <div className="sk-panel border border-glass-edge bg-glass backdrop-glass shadow-glass grid gap-4 rounded-[18px] p-5">{label}<SkeletonRows /></div>
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
        title: "Your session has ended",
        message: "Sign in again to continue. Your session may have expired or been ended by an administrator.",
        statusText: "401 Unauthorized",
      };
    }
    if (error.status === 403) {
      return {
        kind: "auth",
        title: "You don't have access to this page",
        message: "Your account doesn't have permission for this view. Ask a project owner or administrator to check your access.",
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
        hint: "go -C backend run ./cmd/tracepilot serve",
      };
    }
    if (error.status === 503 || error.status === 504) {
      return {
        kind: "network",
        title: "TracePilot is busy",
        message: "The service is temporarily overloaded. Wait a few seconds and try again.",
        statusText: `HTTP ${error.status}`,
        hint: "go -C backend run ./cmd/tracepilot serve",
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
      hint: "go -C backend run ./cmd/tracepilot serve",
    };
  }
  return { kind: "error", title: "We couldn't load this view", message };
}

const noticeIcons = { auth: KeyRound, network: WifiOff, error: AlertTriangle } as const;

/** One notification layout for every error: centred icon, short title, plain message, actions, then collapsed details. */
export function Notice({ guidance, variant = "inline", actions }: { guidance: ErrorGuidance; variant?: "inline" | "page" | "banner"; actions?: ReactNode }) {
  const Icon = noticeIcons[guidance.kind];
  const { statusText, detail, hint } = guidance;
  const banner = variant === "banner";
  return (
    <div className={`notice notice-${variant} notice-${guidance.kind} mx-auto flex w-full gap-3 rounded-2xl border border-glass-edge bg-glass p-6 text-text backdrop-glass shadow-glass ${guidance.kind === "auth" ? "[--tone:var(--warning)]" : "[--tone:var(--danger)]"} ${banner ? "mb-4 max-w-[760px] flex-wrap items-center text-left" : "my-auto max-w-[560px] flex-col items-center text-center in-[.panel]:my-5 in-[.panel]:border-0 in-[.panel]:bg-transparent in-[.panel]:shadow-none in-[.panel]:backdrop-filter-none"}`} role="alert">
      <span className={`notice-icon inline-flex shrink-0 items-center justify-center rounded-xl border border-(--tone)/30 bg-(--tone)/15 text-(--tone) ${banner ? "size-9" : "size-12"}`} aria-hidden="true"><Icon size={banner ? 18 : 22} /></span>
      <div className={`notice-body min-w-0 ${banner ? "flex-1" : "w-full"}`}>
        <strong className="notice-title block text-[15px] leading-snug font-semibold">{guidance.title}</strong>
        <p className="notice-text mt-1 text-[13px] leading-relaxed text-muted">{guidance.message}</p>
        {(statusText || detail || hint) && (
          <details className="notice-details mt-2.5 w-full text-left text-xs text-faint open:[&>summary]:mb-1.5">
            <summary className="mx-auto w-fit cursor-pointer rounded-md px-1 py-0.5 font-semibold select-none hover:bg-glass-hover hover:text-text">Technical details</summary>
            {statusText && <p className="mb-1 wrap-anywhere">{statusText}</p>}
            {detail && <p className="mb-1 wrap-anywhere">{detail}</p>}
            {hint && <div className="notice-hint mt-1.5 flex items-center justify-between gap-2 rounded-lg border border-glass-divider bg-glass-sunken p-1.5 pl-2.5"><code className="min-w-0 truncate text-text select-all">{hint}</code><CopyButton value={hint} label="Copy command" /></div>}
          </details>
        )}
      </div>
      {actions && <div className="notice-actions flex flex-wrap items-center justify-center gap-2">{actions}</div>}
    </div>
  );
}

export function ErrorState({ error, retry, fullPage }: { error: unknown; retry?: () => void; fullPage?: boolean }) {
  const guidance = errorGuidance(error);
  const retryable = !(error instanceof APIError && (error.status === 401 || error.status === 403));
  const actions = (
    <>
      {retry && retryable && <button type="button" className="notice-btn inline-flex h-8.5 items-center justify-center gap-1.5 rounded-full bg-btn px-4 text-xs font-semibold text-btn-text hover:bg-btn-hover" onClick={retry}><RefreshCw size={13} aria-hidden="true" /> Retry</button>}
      {fullPage && <a href="#/flow" className="notice-btn ghost inline-flex h-8.5 items-center justify-center rounded-full border border-glass-field-edge bg-glass-field px-4 text-xs font-semibold no-underline hover:bg-glass-field-focus">How it works</a>}
    </>
  );
  return <Notice guidance={guidance} variant={fullPage ? "page" : "inline"} actions={actions} />;
}

// ── Utility controls ───────────────────────────────────────────────────────────

/** Copies text and reports success or failure; clipboard access can be denied. */
export function CopyButton({ value, label, compact = false }: { value: string; label: string; compact?: boolean }) {
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
    <span className="copy-control inline-flex items-center gap-1">
      <button type="button" className={compact ? "icon-button inline-flex size-7 shrink-0 items-center justify-center rounded-md text-muted transition-colors hover:bg-hover hover:text-text" : `icon-button ${secondaryButtonClass} px-2 py-1`} aria-label={label} title={state === "failed" ? "Clipboard access was denied. Select the text and copy it manually." : label} onClick={() => void copy()}>
        {state === "copied" ? <Check size={16} /> : state === "failed" ? <AlertTriangle size={16} className="text-danger-fg" /> : <Copy size={16} />}
      </button>
      <span role="status" className={state === "failed" && !compact ? "form-error mt-1.5 text-[12.5px] font-medium leading-relaxed text-danger-fg" : "sr-only"}>
        {state === "copied" ? "Copied" : state === "failed" ? "Clipboard access was denied. Select the text and copy it manually." : ""}
      </span>
    </span>
  );
}

/** Long values stay scannable: collapsed by default, expandable in place. */
export function ExpandableText({ value, limit = 360 }: { value: string; limit?: number }) {
  const [open, setOpen] = useState(false);
  if (value.length <= limit) return <pre className="inline-json m-0 max-h-48 max-w-full overflow-auto whitespace-pre-wrap wrap-anywhere font-mono text-[11px] text-code">{value}</pre>;
  return (
    <div className="expandable">
      <pre className="inline-json m-0 max-h-48 max-w-full overflow-auto whitespace-pre-wrap wrap-anywhere font-mono text-[11px] text-code">{open ? value : `${value.slice(0, limit)}…`}</pre>
      <button type="button" className={`button-secondary ${secondaryButtonClass} compact px-2.5 py-1 text-[11.5px]`} aria-expanded={open} onClick={() => setOpen(!open)}>
        {open ? "Show less" : `Show all (${value.length.toLocaleString()} chars)`}
      </button>
    </div>
  );
}
