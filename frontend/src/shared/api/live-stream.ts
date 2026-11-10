import { useEffect, useRef, useState } from "react";
import type { LogRecord } from "../types";
import { APIError, apiResponse, requestScopeKey } from "./client";

export type LiveStreamOptions = {
  enabled: boolean;
  credential: string;
  onLog?: (log: LogRecord) => void;
  onIncident?: (incident: Record<string, unknown>) => void;
};

export function useLiveStream({ enabled, credential, onLog, onIncident }: LiveStreamOptions) {
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const scope = requestScopeKey();
  const onLogRef = useRef(onLog);
  const onIncidentRef = useRef(onIncident);
  onLogRef.current = onLog;
  onIncidentRef.current = onIncident;

  useEffect(() => {
    if (!enabled || !credential) {
      setConnected(false);
      setError(null);
      return;
    }

    const abortController = new AbortController();
    let isCancelled = false;
    let retryTimer = 0;
    let wakeRetry: (() => void) | undefined;
    let failures = 0;
    setError(null);

    async function connect() {
      while (!isCancelled) {
        try {
          const response = await apiResponse("/api/v1/live/stream", credential, {
            headers: {
              Accept: "text/event-stream",
            },
            signal: abortController.signal,
          });
          if (isCancelled) return;

          if (!response.body) throw new Error("The live stream has no response body.");
          {
            setConnected(true);
            setError(null);
            failures = 0;
            const reader = response.body.getReader();
            const decoder = new TextDecoder();
            let buffer = "";

            while (!isCancelled) {
              const { done, value } = await reader.read();
              if (isCancelled) return;
              if (done) break;

              buffer = (buffer + decoder.decode(value, { stream: true })).replace(/\r\n/g, "\n");
              const blocks = buffer.split("\n\n");
              buffer = blocks.pop() || "";

              for (const block of blocks) {
                const dataMatch = block.match(/(?:^|\n)data:\s*(.+)$/m);
                if (dataMatch) {
                  try {
                    const parsed = JSON.parse(dataMatch[1]);
                    if (parsed.type === "log" && onLogRef.current) {
                      onLogRef.current(parsed.payload as LogRecord);
                    } else if (parsed.type === "incident" && onIncidentRef.current) {
                      onIncidentRef.current(parsed.payload as Record<string, unknown>);
                    }
                  } catch {
                    // Ignore heartbeats or invalid JSON chunks
                  }
                }
              }
            }
          }
        } catch (err: unknown) {
          if (isCancelled || (err instanceof DOMException && err.name === "AbortError")) return;
          setConnected(false);
          setError(err instanceof Error ? err : new Error("The live stream disconnected."));
          if (err instanceof APIError && err.status >= 400 && err.status < 500 && err.status !== 429) return;
        }

        setConnected(false);
        if (isCancelled) return;
        await new Promise<void>((resolve) => {
          wakeRetry = resolve;
          retryTimer = window.setTimeout(resolve, Math.min(1_000 * 2 ** failures++, 15_000));
        });
      }
    }

    void connect();

    return () => {
      isCancelled = true;
      window.clearTimeout(retryTimer);
      wakeRetry?.();
      abortController.abort();
      setConnected(false);
    };
  }, [enabled, credential, scope]);

  return { connected, error };
}
