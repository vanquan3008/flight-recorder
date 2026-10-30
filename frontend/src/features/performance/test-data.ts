import type { RequestPerformance } from "../../shared/types";

export const emptyPerformance: RequestPerformance = {
  from: "2026-10-01T00:00:00.000Z", to: "2026-10-01T01:00:00.000Z",
  service: "", endpoint: "", bucket_seconds: 60, limit: 50,
  summary: { requests: 0, errors: 0, requests_per_second: 0, error_rate: 0, average_latency_nanos: 0, p95_latency_nanos: 0, p99_latency_nanos: 0 },
  series: [], services: [], endpoints: [],
};
