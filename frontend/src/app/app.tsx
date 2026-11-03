import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { HashRouter } from "react-router-dom";
import { AppShell } from "./app-shell";

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
  return <QueryClientProvider client={queryClient}><HashRouter><AppShell /></HashRouter></QueryClientProvider>;
}
