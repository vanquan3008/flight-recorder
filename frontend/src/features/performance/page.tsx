import { useQuery } from "@tanstack/react-query";
import { useAuth } from "../../shared/session";
import { api, queryString } from "../../shared/api";
import type { TelemetryQuality } from "../../shared/types";
import { PageHeader, TimeWindowFilter, useTimeWindow } from "../../shared/ui";
import { PerformanceDashboard } from "./dashboard";

export function PerformancePage() {
  const { credential } = useAuth();
  const window = useTimeWindow();
  const qualityPath = `/api/v1/telemetry/quality?${queryString({ from: window.from, to: window.to })}`;
  // The quality donut is optional: the dashboard still renders when this call fails.
  const quality = useQuery({ queryKey: ["overview", qualityPath], queryFn: ({ signal }) => api<TelemetryQuality>(qualityPath, credential, { signal }) });
  return (
    <div className="performance-page flex h-full max-h-full min-h-0 w-full max-w-full flex-1 flex-col overflow-hidden">
      <div className="shrink-0">
        <PageHeader title="Performance" action={<TimeWindowFilter {...window} />}>
          Inbound request traffic, latency and errors, by service and endpoint.
        </PageHeader>
      </div>
      <PerformanceDashboard from={window.from} to={window.to} quality={quality.data} />
    </div>
  );
}
