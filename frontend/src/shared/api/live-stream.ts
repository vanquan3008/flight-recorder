import { useEffect, useRef, useState } from "react";
import type { LogRecord } from "../types";

export type LiveStreamOptions = {
  enabled: boolean;
  credential: string;
  onLog?: (log: LogRecord) => void;
  onIncident?: (incident: Record<string, unknown>) => void;
};

export function useLiveStream({ enabled, credential, onLog, onIncident }: LiveStreamOptions) {
  const [connected, setConnected] = useState(false);
  const onLogRef = useRef(onLog);
  const onIncidentRef = useRef(onIncident);
  onLogRef.current = onLog;
  onIncidentRef.current = onIncident;

  useEffect(() => {
    if (!enabled || !credential) {
      setConnected(false);
      return;
    }

    const abortController = new AbortController();
    let isCancelled = false;
    let retryTimer = 0;

    async function connect() {
      while (!isCancelled) {
        try {
          const response = await fetch("/api/v1/live/stream", {
            headers: {
              Authorization: `Bearer ${credential}`,
              Accept: "text/event-stream",
            },
            signal: abortController.signal,
          });

          if (!response.ok || !response.body) {
            setConnected(false);
            if (response.status >= 400 && response.status < 500 && response.status !== 429) return;
          } else {
            setConnected(true);
            const reader = response.body.getReader();
            const decoder = new TextDecoder();
            let buffer = "";

            while (!isCancelled) {
              const { done, value } = await reader.read();
              if (done) break;

              buffer += decoder.decode(value, { stream: true });
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
        }

        setConnected(false);
        if (isCancelled) return;
        await new Promise<void>((resolve) => {
          retryTimer = window.setTimeout(resolve, 1_000);
        });
      }
    }

    void connect();

    return () => {
      isCancelled = true;
      window.clearTimeout(retryTimer);
      abortController.abort();
      setConnected(false);
    };
  }, [enabled, credential]);

  return { connected };
}
