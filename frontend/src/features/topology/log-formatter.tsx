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
      <code className="log-line-code" title={text}>
        <span className="log-raw truncate">{text}</span>
      </code>
    );
  }

  const [, method, path, statusCode, duration, unit, extra] = match;
  const statusNum = Number(statusCode);
  const methodUpper = method.toUpperCase();

  const methodClass =
    methodUpper === "GET"
      ? "method-get"
      : methodUpper === "POST"
      ? "method-post"
      : methodUpper === "PUT" || methodUpper === "PATCH"
      ? "method-put"
      : methodUpper === "DELETE"
      ? "method-delete"
      : "method-other";

  const statusClass =
    statusNum >= 500
      ? "status-5xx"
      : statusNum >= 400
      ? "status-4xx"
      : statusNum >= 300
      ? "status-3xx"
      : "status-2xx";

  return (
    <code className="log-line-code" title={text}>
      <span className={`method-badge ${methodClass}`}>{methodUpper}</span>
      <span className="log-path font-mono truncate" title={path}>
        {path}
      </span>
      <span className={`status-badge ${statusClass}`}>{statusCode}</span>
      <span className="log-duration font-mono shrink-0">
        {formatDuration(duration, unit)}
      </span>
      {extra?.trim() && (
        <span className="log-extra truncate" title={extra.trim()}>
          {extra.trim()}
        </span>
      )}
    </code>
  );
}
