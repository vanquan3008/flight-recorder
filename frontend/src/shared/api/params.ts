/** Builds a query string with a default limit=50 and omits blank values. */
export function queryString(values: Record<string, string | undefined>): string {
  const query = new URLSearchParams({ limit: "50" });
  Object.entries(values).forEach(([key, value]) => {
    if (value?.trim()) query.set(key, value.trim());
  });
  return query.toString();
}

/** Reads the non-empty values from a form into URLSearchParams, preserving nominated carry-over keys. */
export function compactSearchParams(form: HTMLFormElement, current?: URLSearchParams, preserve: string[] = []): URLSearchParams {
  const params = new URLSearchParams();
  new FormData(form).forEach((value, key) => {
    const normalized = String(value).trim();
    if (normalized) params.set(key, normalized);
  });
  for (const key of ["project", "environment", ...preserve]) {
    const value = current?.get(key);
    if (value) params.set(key, value);
  }
  return params;
}
