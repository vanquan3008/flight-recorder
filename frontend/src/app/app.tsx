import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { BrowserRouter, useLocation, useNavigate } from "react-router-dom";
import { AppShell } from "./shell/app-shell";

/** Synchronizes and cleans legacy hash URLs (e.g. `/#/settings?tab=projects` -> `/settings?tab=projects`). */
function HashRedirect() {
  const navigate = useNavigate();
  const location = useLocation();
  useEffect(() => {
    if (window.location.hash.startsWith("#/")) {
      const target = window.location.hash.slice(1);
      window.location.hash = "";
      navigate(target, { replace: true });
    }
  }, [location, navigate]);
  return null;
}

export function App() {
  const [queryClient] = useState(() => new QueryClient({
    defaultOptions: {
      queries: {
        // Client errors and 501 (feature disabled) are permanent; retry transient server/network failures with backoff.
        retry: (count, error) => !(error instanceof Error && "status" in error && ((error as { status: number }).status < 500 || (error as { status: number }).status === 501)) && count < 2,
        retryDelay: (attemptIndex) => Math.min(500 * (2 ** attemptIndex), 3000),
        staleTime: 15_000,
      },
    },
  }));

  if (typeof window !== "undefined" && window.location.hash.startsWith("#/")) {
    const target = window.location.hash.slice(1);
    window.history.replaceState(null, "", target);
    window.location.hash = "";
  }

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <HashRedirect />
        <AppShell />
      </BrowserRouter>
    </QueryClientProvider>
  );
}
