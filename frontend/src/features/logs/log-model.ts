import type { LogRecord } from "../../shared/types";
import { logBodyText } from "../../shared/ui/formatters";

export function matchesLogFilters(log: LogRecord, params: URLSearchParams): boolean {
  const fields = { service: log.service_name, severity: log.severity_text, traceId: log.trace_id, spanId: log.span_id };
  if (Object.entries(fields).some(([key, value]) => params.get(key) && params.get(key) !== value)) return false;
  const search = params.get("search");
  return !search || logBodyText(log.body).toLocaleLowerCase().includes(search.toLocaleLowerCase());
}

export function mergeLogs(live: LogRecord[], historical: LogRecord[]): LogRecord[] {
  const ids = new Set(live.map((log) => log.event_id));
  return [...live, ...historical.filter((log) => !ids.has(log.event_id))];
}
