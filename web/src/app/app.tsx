import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { HashRouter } from "react-router-dom";
import { AppShell } from "./app-shell";

export function App() {
  const [queryClient] = useState(() => new QueryClient({
    defaultOptions: {
      queries: {
        retry: (count, error) => !(error instanceof Error && "status" in error && (error as { status: number }).status < 500) && count < 1,
        staleTime: 15_000,
      },
    },
  }));
  return <QueryClientProvider client={queryClient}><HashRouter><AppShell /></HashRouter></QueryClientProvider>;
}
