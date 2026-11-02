import { useQuery } from "@tanstack/react-query";
import { useAuth } from "../../app/auth";
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
  return <>
    <PageHeader title="Performance" action={<TimeWindowFilter range={window.range} select={window.select} />}>Inbound request traffic, latency and errors, by service and endpoint.</PageHeader>
    <PerformanceDashboard from={window.from} to={window.to} quality={quality.data} />
  </>;
}
