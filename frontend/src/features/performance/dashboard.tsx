import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { useAuth } from "../../app/auth";
import { api, queryString } from "../../shared/api";
import { Sparkline } from "../../shared/charts";
import type { PerformanceBucket, PerformanceStats, RequestPerformance, TelemetryQuality } from "../../shared/types";
import { DataTable, DetailLink, ErrorState, Panel, formatDuration } from "../../shared/ui";
import { QualityDonut, ServiceErrorBars } from "./insights";
import { PerformanceSkeleton } from "./skeleton";
import { PerformanceTrend } from "./trend";
import "./performance.css";

export const formatRate = (value: number) => value.toLocaleString("en", { maximumSignificantDigits: 3 });
const percent = (value: number) => `${(value * 100).toFixed(2)}%`;
const latency = (requests: number, nanos: number) => requests ? (nanos === 0 ? "0ms" : formatDuration(nanos)) : "—";
const columns = (stats: PerformanceStats) => [
  stats.requests.toLocaleString(), formatRate(stats.requests_per_second), stats.errors.toLocaleString(),
  stats.requests ? percent(stats.error_rate) : "—", latency(stats.requests, stats.average_latency_nanos),
  latency(stats.requests, stats.p95_latency_nanos), latency(stats.requests, stats.p99_latency_nanos),
];
const serviceLink = (service: string, from: string, to: string) =>
  <DetailLink to={`/traces?${queryString({ service, from, to })}`}>{service}</DetailLink>;
const headers = ["Requests", "RPS", "Errors", "Error rate", "Avg", "P95", "P99"];

export function PerformanceDashboard({ from, to, quality }: { from: string; to: string; quality?: TelemetryQuality }) {
  const { credential } = useAuth();
  const [params, setParams] = useSearchParams();
  const service = params.get("performanceService") || "";
  const endpoint = params.get("performanceEndpoint") || "";
  const path = `/api/v1/performance?${queryString({ from, to, service, endpoint })}`;
  const query = useQuery({ queryKey: ["performance", path], queryFn: ({ signal }) => api<RequestPerformance>(path, credential, { signal }), placeholderData: keepPreviousData });
  const applyFilters = (values: FormData) => {
    const next = new URLSearchParams(params);
    for (const name of ["Service", "Endpoint"]) {
      const value = String(values.get(name.toLowerCase()) || "").trim();
      if (value) next.set(`performance${name}`, value);
      else next.delete(`performance${name}`);
    }
    setParams(next);
  };
  const data = query.data;
  const summary = data?.summary;
  const trend = (value: (bucket: PerformanceBucket) => number) => (data?.series ?? []).map(value);
  const cards = summary ? [
    { label: "Observed requests", value: summary.requests.toLocaleString(), hint: "inbound across selected services", spark: trend((b) => b.requests), tone: "var(--accent)" },
    { label: "Request rate", value: formatRate(summary.requests_per_second), hint: "req/s · full selected window", spark: trend((b) => b.requests_per_second), tone: "var(--violet)" },
    { label: "Request error rate", value: summary.requests ? percent(summary.error_rate) : "—", hint: "span ERROR or HTTP 5xx", spark: trend((b) => b.error_rate), tone: "var(--danger)" },
    { label: "Average latency", value: latency(summary.requests, summary.average_latency_nanos), hint: "inbound requests only", spark: trend((b) => b.average_latency_nanos), tone: "var(--cyan)" },
    { label: "P95 latency", value: latency(summary.requests, summary.p95_latency_nanos), hint: "95th percentile of requests", spark: trend((b) => b.p95_latency_nanos), tone: "var(--accent)" },
    { label: "P99 latency", value: latency(summary.requests, summary.p99_latency_nanos), hint: "99th percentile of requests", spark: trend((b) => b.p99_latency_nanos), tone: "var(--warning)" },
  ] : [];
  return <Panel title="Traffic and performance" className="performance-dashboard" meta="Observed inbound requests">
    <p className="performance-note">Counts and latency use received SERVER spans. Sampling or missing telemetry can undercount traffic and bias percentiles. A request crossing services is counted once at each receiving service.</p>
    <form className="toolbar performance-filters" key={`${service}|${endpoint}`} onSubmit={(event) => { event.preventDefault(); applyFilters(new FormData(event.currentTarget)); }}>
      <label>Service <input aria-label="Performance service" name="service" defaultValue={service} placeholder="All services" maxLength={4096} /></label>
      <label>Endpoint <input aria-label="Performance endpoint" name="endpoint" defaultValue={endpoint} placeholder="e.g. GET /orders" maxLength={4096} /></label>
      <button type="submit">Apply performance filters</button>
      {(service || endpoint) && <button type="button" onClick={() => applyFilters(new FormData())}>Clear filters</button>}
    </form>
    {query.isPending ? <PerformanceSkeleton /> : query.isError ? <ErrorState error={query.error} retry={() => void query.refetch()} /> : data && <div className={`performance-content${query.isPlaceholderData ? " is-stale" : ""}`} aria-busy={query.isPlaceholderData}>
      <div className="stat-grid performance-stats">{cards.map((card) => <div className="stat-card" key={card.label}>
        <span className="stat-label">{card.label}</span>
        <div className="stat-main"><strong className="stat-value">{card.value}</strong>{summary?.requests ? <Sparkline values={card.spark} tone={card.tone} /> : null}</div>
        <span className="stat-hint">{card.hint}</span>
      </div>)}</div>
      <div className="performance-trends">
        <PerformanceTrend series={data.series} kind="rate" />
        <PerformanceTrend series={data.series} kind="latency" />
      </div>
      <div className={`performance-insights${quality ? "" : " single"}`}>
        <section><h3>Services by errors <span>Top 6 · request errors in the window</span></h3>
          <ServiceErrorBars services={data.services} from={from} to={to} /></section>
        {quality && <section><h3>Telemetry quality <span>Can logs and spans be followed?</span></h3><QualityDonut quality={quality} /></section>}
      </div>
      <div className="performance-breakdown"><h3>By endpoint <span>Top {data.limit} · templated HTTP route, RPC method, or span name</span></h3>
        <DataTable headers={["Service", "Endpoint", ...headers]} empty="No endpoints observed in this window."
          rows={data.endpoints.map((row) => [serviceLink(row.service, from, to), row.endpoint, ...columns(row)])} />
      </div>
    </div>}
  </Panel>;
}
