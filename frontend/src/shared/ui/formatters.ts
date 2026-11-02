/** Formats an ISO timestamp into a medium date + short time string. Returns "—" for missing values. */
export const formatTime = (value?: string): string =>
  value
    ? new Date(value).toLocaleString([], { dateStyle: "medium", timeStyle: "short" })
    : "—";

/** Formats a nanosecond duration into a human-readable µs / ms / s string. Returns "—" for falsy values. */
export const formatDuration = (nanoseconds?: number): string => {
  if (!nanoseconds) return "—";
  if (nanoseconds < 1e6) return `${(nanoseconds / 1e3).toFixed(1)}µs`;
  if (nanoseconds < 1e9) return `${(nanoseconds / 1e6).toFixed(1)}ms`;
  return `${(nanoseconds / 1e9).toFixed(2)}s`;
};

/** Pretty-prints unknown JSON; if the value is already a JSON string, re-indents it. */
export const json = (value: unknown): string => {
  if (typeof value === "string") {
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return value;
    }
  }
  return JSON.stringify(value ?? {}, null, 2);
};

/**
 * Formats a log body for display. Short structured objects stay on one line so many logs fit on
 * screen; longer values use the indented form.
 */
export const logBodyText = (body: unknown): string => {
  const compact = typeof body === "string" ? undefined : JSON.stringify(body);
  return compact !== undefined && compact.length <= 160 ? compact : json(body);
};
