import type { Span } from "../../shared/types";
import type { TraceModel } from "./trace-model";
import { formatMs, isError } from "./trace-model";

export type SpanDisplayInfo = {
  method?: string;
  target?: string;
  displayName: string;
  httpStatus?: number;
  errorType?: string;
  isClientCall: boolean;
  isDbCall: boolean;
};

/**
 * Shortens a full URL into host + truncated path (e.g. s3.host.com/.../file.png)
 */
export function formatShortUrl(rawUrl: string): string {
  try {
    const parsed = new URL(rawUrl);
    const path = parsed.pathname;
    if (path.length <= 32) {
      return `${parsed.host}${path}`;
    }
    const segments = path.split("/").filter(Boolean);
    if (segments.length <= 1) {
      return `${parsed.host}/${path.slice(0, 16)}...${path.slice(-10)}`;
    }
    const last = segments[segments.length - 1];
    return `${parsed.host}/.../${last}`;
  } catch {
    if (rawUrl.length > 36) {
      return `${rawUrl.slice(0, 20)}...${rawUrl.slice(-12)}`;
    }
    return rawUrl;
  }
}

/**
 * Extracts clean, human-readable display information from span attributes.
 */
export function getSpanDisplayInfo(span: Span): SpanDisplayInfo {
  const attrs = (span.attributes as Record<string, unknown>) || {};
  
  const rawMethod = (attrs["http.request.method"] || attrs["http.method"]) as string | undefined;
  const urlFull = attrs["url.full"] as string | undefined;
  const urlPath = (attrs["url.path"] || attrs["http.target"]) as string | undefined;
  const httpRoute = attrs["http.route"] as string | undefined;
  const serverAddress = (attrs["server.address"] || attrs["net.peer.name"]) as string | undefined;
  const rawStatus = attrs["http.response.status_code"] || attrs["http.status_code"];
  const httpStatus = typeof rawStatus === "number" ? rawStatus : typeof rawStatus === "string" ? parseInt(rawStatus, 10) : undefined;
  const errorType = (attrs["error.type"] || attrs["exception.type"]) as string | undefined;
  
  const dbSystem = attrs["db.system"] as string | undefined;
  const dbStatement = attrs["db.statement"] as string | undefined;
  const isDbCall = Boolean(dbSystem || dbStatement);
  const isClientCall = Boolean(urlFull || (serverAddress && serverAddress !== "localhost" && serverAddress !== "127.0.0.1"));

  let method = rawMethod;
  if (!method && span.name) {
    const match = span.name.match(/^(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS|CONNECT|TRACE)\b/i);
    if (match) {
      method = match[1].toUpperCase();
    } else if (isDbCall) {
      method = "DB";
    }
  }

  let target: string | undefined;
  if (urlFull) {
    target = formatShortUrl(urlFull);
  } else if (httpRoute) {
    target = httpRoute;
  } else if (urlPath) {
    target = urlPath;
  } else if (serverAddress) {
    target = serverAddress;
  } else if (dbStatement) {
    target = dbStatement.length > 40 ? `${dbStatement.slice(0, 37)}...` : dbStatement;
  }

  // If the span name is already informative (e.g. "GET /api/users" or "SELECT orders"), keep it as is.
  let displayName = span.name;
  const isBareMethod = /^(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS|HTTP GET|HTTP POST)$/i.test(span.name.trim());
  if (isBareMethod && target) {
    displayName = `${method || span.name} ${target}`;
  }

  return {
    method,
    target,
    displayName,
    httpStatus: Number.isFinite(httpStatus) ? httpStatus : undefined,
    errorType,
    isClientCall,
    isDbCall,
  };
}

export type TraceInsight = {
  type: "error" | "warning" | "info";
  title: string;
  description: string;
};

/**
 * Analyses trace rows to surface high-value insights (root cause of errors, retries, slow dependencies).
 */
export function analyzeTraceInsights(model: TraceModel): TraceInsight[] {
  const insights: TraceInsight[] = [];
  const errorRows = model.rows.filter((row) => isError(row.span));

  if (errorRows.length > 0) {
    // Check if multiple failed spans targeted the same endpoint/host (e.g. S3 retry storm)
    const clientErrors = new Map<string, { count: number; service: string; status?: number; target: string; totalMs: number }>();
    
    for (const row of errorRows) {
      const info = getSpanDisplayInfo(row.span);
      if (info.isClientCall || info.target) {
        const key = `${row.span.service_name}|${info.target || row.span.name}|${info.httpStatus || info.errorType || "ERR"}`;
        const existing = clientErrors.get(key) || {
          count: 0,
          service: row.span.service_name,
          status: info.httpStatus,
          target: info.target || row.span.name,
          totalMs: 0,
        };
        existing.count += 1;
        existing.totalMs += row.durationMs;
        clientErrors.set(key, existing);
      }
    }

    let detectedPattern = false;
    for (const item of clientErrors.values()) {
      if (item.count >= 2) {
        detectedPattern = true;
        const statusText = item.status ? `HTTP ${item.status}` : "errors";
        insights.push({
          type: "error",
          title: `Detected retry failure pattern (${item.count} calls failed)`,
          description: `${item.count} outbound requests from ${item.service} to ${item.target} failed with ${statusText}. Repeated retries contributed to total request latency.`,
        });
      }
    }

    if (!detectedPattern) {
      insights.push({
        type: "error",
        title: `${errorRows.length} ${errorRows.length === 1 ? "span failed" : "spans failed"} during execution`,
        description: `Inspect the highlighted red spans below to view error codes, messages, and stack traces.`,
      });
    }
  }

  // Highlight if one span dominated >75% of the total request time
  if (model.rows.length > 1 && model.totalMs > 1000) {
    const slowest = [...model.rows].sort((a, b) => b.durationMs - a.durationMs)[0];
    if (slowest && slowest.durationMs / model.totalMs >= 0.75) {
      const info = getSpanDisplayInfo(slowest.span);
      insights.push({
        type: "warning",
        title: `Latency bottleneck: ${formatMs(slowest.durationMs)} (${Math.round((slowest.durationMs / model.totalMs) * 100)}% of total)`,
        description: `Span "${info.displayName}" in ${slowest.span.service_name} accounted for the majority of the request duration.`,
      });
    }
  }

  return insights;
}
