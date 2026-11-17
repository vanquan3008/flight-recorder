import type { LogRecord } from "../../shared/types";
import { logBodyText } from "../../shared/ui/formatters";
import { normalizeSeverityText, severityGroups } from "./log-format";

export function matchesLogFilters(log: LogRecord, params: URLSearchParams): boolean {
  const fields = { service: log.service_name, traceId: log.trace_id, spanId: log.span_id };
  if (Object.entries(fields).some(([key, value]) => params.get(key) && params.get(key) !== value)) return false;
  const severity = params.get("severity");
  if (severity) {
    const group = normalizeSeverityText(severity);
    if (group !== normalizeSeverityText(log.severity_text, severityGroups.includes(group) ? log.severity_number : undefined)) return false;
  }
  const search = params.get("search");
  return !search || logBodyText(log.body).toLocaleLowerCase().includes(search.toLocaleLowerCase());
}

export function mergeLogs(live: LogRecord[], historical: LogRecord[]): LogRecord[] {
  const ids = new Set(live.map((log) => log.event_id));
  return [...live, ...historical.filter((log) => !ids.has(log.event_id))];
}
