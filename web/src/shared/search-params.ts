export function compactSearchParams(form: HTMLFormElement, current?: URLSearchParams, preserve: string[] = []): URLSearchParams {
  const params = new URLSearchParams();
  new FormData(form).forEach((value, key) => {
    const normalized = String(value).trim();
    if (normalized) params.set(key, normalized);
  });
  for (const key of preserve) {
    const value = current?.get(key);
    if (value) params.set(key, value);
  }
  return params;
}
