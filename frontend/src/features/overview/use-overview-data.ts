import { useQueries, useQuery } from "@tanstack/react-query";
import { api, queryString } from "../../shared/api";
import { useAuth } from "../../shared/session";
import type { Incident, Page, RequestPerformance, TelemetryQuality, TraceSummary } from "../../shared/types";

const latest = <T,>(path: string, credential: string) => ({
  queryKey: ["overview", path],
  queryFn: ({ signal }: { signal: AbortSignal }) => api<Page<T>>(path, credential, { signal }),
});

export function useOverviewData(from: string, to: string) {
  const { credential } = useAuth();
  const filters = queryString({ from, to });
  const qualityPath = `/api/v1/telemetry/quality?${filters}`;
  const performancePath = `/api/v1/performance?${filters}`;

  const [incidents, traces] = useQueries({
    queries: [
      latest<Incident>(`/api/v1/incidents?${filters}`, credential),
      latest<TraceSummary>(`/api/v1/traces?${filters}`, credential),
    ],
  });

  const quality = useQuery({
    queryKey: ["overview", qualityPath],
    queryFn: ({ signal }: { signal: AbortSignal }) => api<TelemetryQuality>(qualityPath, credential, { signal }),
  });

  const performance = useQuery({
    queryKey: ["performance", performancePath],
    queryFn: ({ signal }) => api<RequestPerformance>(performancePath, credential, { signal }),
  });

  const isPending = incidents.isPending || traces.isPending;
  const error = incidents.isError ? incidents.error : traces.isError ? traces.error : null;
  const refetch = () => {
    [incidents, traces, quality, performance].forEach((q) => void q.refetch());
  };

  return {
    credential,
    filters,
    incidents: incidents.data?.data ?? [],
    traces: traces.data?.data ?? [],
    hasNextIncident: Boolean(incidents.data?.next_cursor),
    metricPoints: quality.isError ? "—" : (quality.data?.metric_points ?? 0),
    performance: performance.data,
    performancePending: performance.isPending,
    isPending,
    error,
    refetch,
  };
}

