import { logBodyText } from "../../shared/ui";

// Regex matching HTTP access logs like:
// HTTP Inbound "GET" "/api/v1/chats/unread**" returned 200 in 41.3ms Remote IP: "::1" Request Headers: ...
// HTTP "GET" "/api/v1/chats/unread" responded 200 in 37.8193 ms
// GET /api/v1/chats 200 in 15ms
const httpPattern =
  /^(?:HTTP\s+(?:Inbound\s+|Outbound\s+)?|\[HTTP\]\s*)?\"?([A-Z]{3,7})\"?\s+\"?([^\"\s]+)\"?\s+(?:returned|responded|status)?\s*(\d{3})(?:\s+in|\s+took)?\s+([\d.]+)\s*(ms|µs|ns|s)?(.*)$/i;

export function normalizeSeverity(severity?: string): string {
  if (!severity) return "—";
  const upper = severity.toUpperCase();
  if (upper === "INFORMATION") return "INFO";
  return upper;
}

function formatDuration(dur: string, unit?: string): string {
  const num = parseFloat(dur);
  if (isNaN(num)) return `${dur}${unit || "ms"}`;
  const u = unit ? unit.trim() : "ms";
  if (num >= 100) return `${Math.round(num)}${u}`;
  if (num >= 10) return `${num.toFixed(1)}${u}`;
  return `${Number(num.toFixed(2))}${u}`;
}

export function FormattedLogLine({ body }: { body: unknown }) {
  const text = logBodyText(body);
  const match = text.match(httpPattern);

  if (!match) {
    return (
      <code className="log-line-code inline-flex min-w-0 max-w-full items-center gap-2 overflow-hidden font-mono text-[11.5px] leading-snug whitespace-nowrap" title={text}>
        <span className="log-raw truncate text-text ">{text}</span>
      </code>
    );
  }

  const [, method, path, statusCode, duration, unit, extra] = match;
  const statusNum = Number(statusCode);
  const methodUpper = method.toUpperCase();

  const methodClass =
    methodUpper === "GET"
      ? "method-get border-sky-500/25 bg-sky-500/12 text-sky-600 dark:bg-sky-500/20 dark:text-sky-400"
      : methodUpper === "POST"
      ? "method-post border-emerald-500/25 bg-emerald-500/12 text-emerald-600 dark:bg-emerald-500/20 dark:text-emerald-400"
      : methodUpper === "PUT" || methodUpper === "PATCH"
      ? "method-put border-amber-500/25 bg-amber-500/12 text-amber-600 dark:bg-amber-500/20 dark:text-amber-400"
      : methodUpper === "DELETE"
      ? "method-delete border-rose-500/25 bg-rose-500/12 text-rose-600 dark:bg-rose-500/20 dark:text-rose-400"
      : "method-other border-violet-500/25 bg-violet-500/12 text-violet-600 dark:bg-violet-500/20 dark:text-violet-400";

  const statusClass =
    statusNum >= 500
      ? "status-5xx border-rose-500/25 bg-rose-500/12 text-rose-600 dark:bg-rose-500/20 dark:text-rose-400"
      : statusNum >= 400
      ? "status-4xx border-amber-500/25 bg-amber-500/12 text-amber-600 dark:bg-amber-500/20 dark:text-amber-400"
      : statusNum >= 300
      ? "status-3xx border-sky-500/25 bg-sky-500/12 text-sky-600 dark:bg-sky-500/20 dark:text-sky-400"
      : "status-2xx border-emerald-500/25 bg-emerald-500/12 text-emerald-600 dark:bg-emerald-500/20 dark:text-emerald-400";

  return (
    <code className="log-line-code inline-flex min-w-0 max-w-full items-center gap-2 overflow-hidden font-mono text-[11.5px] leading-snug whitespace-nowrap" title={text}>
      <span className={`method-badge inline-flex shrink-0 items-center justify-center rounded border px-1.5 py-0.5 text-[9.5px] leading-tight font-extrabold tracking-wide uppercase ${methodClass}`}>{methodUpper}</span>
      <span className="log-path min-w-0 flex-1 truncate font-semibold text-text font-mono " title={path}>
        {path}
      </span>
      <span className={`status-badge inline-flex shrink-0 items-center justify-center rounded border px-1.5 py-0.5 font-mono text-[10px] leading-tight font-bold ${statusClass}`}>{statusCode}</span>
      <span className="log-duration shrink-0 text-[11px] text-muted font-mono ">
        {formatDuration(duration, unit)}
      </span>
      {extra?.trim() && (
        <span className="log-extra truncate text-[11px] text-faint opacity-70 " title={extra.trim()}>
          {extra.trim()}
        </span>
      )}
    </code>
  );
}
