interface ParsedHeader {
  name: string;
  value: string;
}

export interface ParsedHttpLog {
  isHttp: true;
  direction?: string;
  method: string;
  path: string;
  status: number;
  duration: string;
  remoteIp?: string;
  headers: ParsedHeader[];
  firstLine: string;
  restText?: string;
}

const httpRegex =
  /^(?:HTTP\s+(?:(Inbound|Outbound)\s+)?|\[HTTP\]\s*)?\"?([A-Z]{3,7})\"?\s+(.+?)\s+(?:returned|responded|status)?\s*(\d{3})(?:\s+in|\s+took)?\s+([\d.]+)\s*(ms|µs|ns|s)?/i;

export function parseHttpLog(text: string): ParsedHttpLog | null {
  if (!text || typeof text !== "string") return null;

  const firstLine = text.split(/\r?\n/)[0].trim();
  const match = firstLine.match(httpRegex);
  if (!match) return null;

  const [, direction, method, rawPath, statusCode, duration, unit] = match;

  const fullPath = rawPath.replace(/\"+/g, "").trim();

  // Extract remote IP if present in the rest of text
  const ipMatch = text.match(/Remote IP:\s*\"?([^\r\n\"]+)\"?/i);
  const remoteIp = ipMatch ? ipMatch[1].trim() : undefined;

  // Extract request headers if present
  let headers: ParsedHeader[] = [];
  const headersMatch = text.match(/Request Headers:\s*(\[[\s\S]*?\])(?:\r?\n|$)/i);
  if (headersMatch) {
    try {
      const parsed = JSON.parse(headersMatch[1]);
      if (Array.isArray(parsed)) {
        headers = parsed.flatMap((item) =>
          Object.entries(item).map(([k, v]) => ({ name: k, value: String(v) })),
        );
      }
    } catch {
      // Ignore JSON parse error in headers
    }
  }

  // Compute rest text (lines after the first line)
  const newlineIndex = text.indexOf("\n");
  const restText = newlineIndex !== -1 ? text.slice(newlineIndex + 1).trim() : undefined;

  const parsedDuration = parseFloat(duration);
  const formattedDuration = isNaN(parsedDuration)
    ? `${duration}${unit || "ms"}`
    : `${Number(parsedDuration.toFixed(1))}${unit || "ms"}`;

  return {
    isHttp: true,
    direction: direction || undefined,
    method: method.toUpperCase(),
    path: fullPath,
    status: Number(statusCode),
    duration: formattedDuration,
    remoteIp,
    headers,
    firstLine,
    restText,
  };
}

export const severityGroups = ["TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"];

export function normalizeSeverityText(severity?: string, number?: number): string {
  if (number !== undefined && Number.isInteger(number) && number >= 1 && number <= 24) {
    return severityGroups[Math.floor((number - 1) / 4)];
  }
  const upper = severity?.toUpperCase().trim() || "INFO";
  if (upper === "INFORMATION") return "INFO";
  if (upper === "WARNING") return "WARN";
  return upper.match(/^(TRACE|DEBUG|INFO|WARN|ERROR|FATAL)[1-4]?$/)?.[1] ?? upper;
}
