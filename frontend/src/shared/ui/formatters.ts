const timestampFormat = new Intl.DateTimeFormat(undefined, {
  year: "numeric", month: "short", day: "2-digit", hour: "2-digit", minute: "2-digit",
  second: "2-digit", fractionalSecondDigits: 3, timeZoneName: "short",
});

const timeOnlyFormat = new Intl.DateTimeFormat(undefined, {
  hour: "2-digit", minute: "2-digit", second: "2-digit", fractionalSecondDigits: 3, hour12: false,
});

const dateOnlyFormat = new Intl.DateTimeFormat(undefined, {
  year: "numeric", month: "short", day: "2-digit",
});

const tzOnlyFormat = new Intl.DateTimeFormat(undefined, {
  timeZoneName: "short",
});

const pad = (n: number, len = 2) => String(n).padStart(len, "0");

/** Formats timestamp to a clean, single-line technical ISO string: YYYY-MM-DD HH:mm:ss.SSS */
export const formatTimeIso = (value?: string): string => {
  const date = value ? new Date(value) : undefined;
  if (!date || !Number.isFinite(date.getTime())) return "—";
  const y = date.getFullYear();
  const m = pad(date.getMonth() + 1);
  const d = pad(date.getDate());
  const h = pad(date.getHours());
  const min = pad(date.getMinutes());
  const s = pad(date.getSeconds());
  const ms = pad(date.getMilliseconds(), 3);
  return `${y}-${m}-${d} ${h}:${min}:${s}.${ms}`;
};

interface TimeParts {
  time: string;
  date: string;
  full: string;
}

/**
 * Splits timestamp into distinct clock time and date + timezone components.
 * Ideal for stacked display in log explorers and dense tabular views.
 */
export const formatTimeParts = (value?: string): TimeParts | null => {
  const date = value ? new Date(value) : undefined;
  if (!date || !Number.isFinite(date.getTime())) return null;
  const time = timeOnlyFormat.format(date);
  const dateStr = dateOnlyFormat.format(date);
  const tzPart = tzOnlyFormat.formatToParts(date).find((p) => p.type === "timeZoneName")?.value;
  return {
    time,
    date: tzPart ? `${dateStr} · ${tzPart}` : dateStr,
    full: timestampFormat.format(date),
  };
};

/** Keeps sub-second event order visible and names the browser's local time zone. */
export const formatTime = (value?: string): string => {
  const date = value ? new Date(value) : undefined;
  return date && Number.isFinite(date.getTime()) ? timestampFormat.format(date) : "—";
};

/** Formats a nanosecond duration into a human-readable µs / ms / s string. Returns "—" for falsy values. */
export const formatDuration = (nanoseconds?: number): string => {
  if (!nanoseconds) return "—";
  if (nanoseconds < 1e6) return `${(nanoseconds / 1e3).toFixed(1)}µs`;
  if (nanoseconds < 1e9) return `${(nanoseconds / 1e6).toFixed(1)}ms`;
  return `${(nanoseconds / 1e9).toFixed(2)}s`;
};

/** Formats a 0–1 ratio as a percentage; tiny non-zero ratios keep a second decimal so they do not read as 0.0%. */
export const formatPercent = (ratio: number): string => `${(ratio * 100).toFixed(ratio > 0 && ratio < 0.001 ? 2 : 1)}%`;

/** Formats a count with its noun, such as "1,204 requests" or "1 incident". */
export const formatCount = (value: number, noun: string): string => `${value.toLocaleString("en")} ${noun}${value === 1 ? "" : "s"}`;

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
