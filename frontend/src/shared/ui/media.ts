import { useSyncExternalStore } from "react";

export const mobileQuery = "(max-width: 767px)";

function subscribe(query: string, onChange: () => void) {
  if (typeof window === "undefined" || !window.matchMedia) return () => undefined;
  const list = window.matchMedia(query);
  list.addEventListener("change", onChange);
  return () => list.removeEventListener("change", onChange);
}

/** Tracks a media query. Without `matchMedia` (tests, old browsers) it reports false, so the desktop layout renders. */
export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (onChange) => subscribe(query, onChange),
    () => typeof window !== "undefined" && Boolean(window.matchMedia?.(query).matches),
    () => false,
  );
}

export const useIsMobile = () => useMediaQuery(mobileQuery);
