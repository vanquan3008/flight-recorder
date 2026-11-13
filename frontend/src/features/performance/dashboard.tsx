import { toolbarClass, secondaryButtonClass } from "../../shared/ui/styles";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";
import { useSearchParams } from "react-router-dom";
import { useAuth } from "../../shared/session";
import { api, queryString } from "../../shared/api";
import { Sparkline } from "../../shared/charts";
import type { PerformanceBucket, PerformanceStats, RequestPerformance, TelemetryQuality } from "../../shared/types";
import { DataTable, DetailLink, ErrorState, Panel, formatDuration } from "../../shared/ui";
import { QualityDonut, ServiceErrorBars } from "./insights";
import { PerformanceSkeleton } from "./skeleton";
import { PerformanceTrend } from "./trend";

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
  const tab = params.get("tab") === "endpoints" ? "endpoints" : "overview";

  const switchTab = (nextTab: "overview" | "endpoints") => {
    const next = new URLSearchParams(params);
    if (nextTab === "overview") next.delete("tab");
    else next.set("tab", nextTab);
    setParams(next);
  };

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

  const tabSwitcher = (
    <div
      role="tablist"
      aria-label="Performance views"
      className="inline-flex items-center gap-1 p-1 rounded-xl bg-slate-100 dark:bg-white/5 border border-slate-200/80 dark:border-white/10"
    >
      <button
        type="button"
        role="tab"
        id="tab-overview"
        aria-controls="panel-overview"
        aria-selected={tab === "overview"}
        aria-pressed={tab === "overview"}
        onClick={() => switchTab("overview")}
        className={`px-3 py-1 rounded-lg text-xs font-semibold transition-all cursor-pointer ${
          tab === "overview"
            ? "bg-white dark:bg-sky-500 text-slate-900 dark:text-white shadow-xs"
            : "text-slate-500 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white"
        }`}
      >
        Overview
      </button>
      <button
        type="button"
        role="tab"
        id="tab-endpoints"
        aria-controls="panel-endpoints"
        aria-selected={tab === "endpoints"}
        aria-pressed={tab === "endpoints"}
        onClick={() => switchTab("endpoints")}
        className={`px-3 py-1 rounded-lg text-xs font-semibold transition-all cursor-pointer ${
          tab === "endpoints"
            ? "bg-white dark:bg-sky-500 text-slate-900 dark:text-white shadow-xs"
            : "text-slate-500 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white"
        }`}
      >
        Endpoints {data?.endpoints?.length ? `(${data.endpoints.length})` : ""}
      </button>
    </div>
  );

  return (
    <Panel
      fit
      title={
        tab === "endpoints" ? (
          <div className="flex items-center gap-2.5">
            <button
              type="button"
              onClick={() => switchTab("overview")}
              className="inline-flex items-center justify-center w-7 h-7 rounded-lg border border-slate-200 dark:border-white/10 hover:bg-slate-100 dark:hover:bg-white/10 text-slate-600 dark:text-slate-300 transition-colors cursor-pointer"
              aria-label="Back to Overview"
              title="Back to Overview"
            >
              <ArrowLeft size={15} />
            </button>
            <span>Endpoints breakdown</span>
          </div>
        ) : (
          "Traffic and performance"
        )
      }
      className="performance-dashboard flex min-h-0 flex-1 flex-col overflow-hidden"
      meta={tabSwitcher}
    >
      <p className="performance-note shrink-0 mx-4.5 mt-2.5 mb-2 text-xs leading-snug text-muted">
        {tab === "endpoints"
          ? "Templated HTTP routes, RPC methods, and span names ranked by inbound request volume, error rate, and latency."
          : "Counts and latency use received SERVER spans. Sampling or missing telemetry can undercount traffic and bias percentiles. A request crossing services is counted once at each receiving service."}
      </p>
      <form
        className={`toolbar ${toolbarClass} performance-filters shrink-0 items-end px-4.5 pb-2.5 [&_label]:grid [&_label]:gap-1 [&_label]:text-[11.5px] [&_label]:text-muted max-[620px]:[&_label]:w-full max-[620px]:[&_input]:w-full`}
        key={`${service}|${endpoint}`}
        onSubmit={(event) => {
          event.preventDefault();
          applyFilters(new FormData(event.currentTarget));
        }}
      >
        <label>
          Service{" "}
          <input
            aria-label="Performance service"
            name="service"
            defaultValue={service}
            placeholder="All services"
            maxLength={4096}
          />
        </label>
        <label>
          Endpoint{" "}
          <input
            aria-label="Performance endpoint"
            name="endpoint"
            defaultValue={endpoint}
            placeholder="e.g. GET /orders"
            maxLength={4096}
          />
        </label>
        <button type="submit">Apply performance filters</button>
        {(service || endpoint) && (
          <button type="button" onClick={() => applyFilters(new FormData())}>
            Clear filters
          </button>
        )}
      </form>

      {query.isPending ? (
        <PerformanceSkeleton tab={tab} />
      ) : query.isError ? (
        <ErrorState error={query.error} retry={() => void query.refetch()} />
      ) : (
        data && (
          <div
            className={`performance-content flex min-h-0 flex-1 flex-col overflow-hidden transition-opacity aria-busy:pointer-events-none aria-busy:opacity-50${query.isPlaceholderData ? " is-stale" : ""}`}
            aria-busy={query.isPlaceholderData}
          >
            {/* Overview Tab Panel: Exactly 1 screen view */}
            <div
              role="tabpanel"
              id="panel-overview"
              aria-labelledby="tab-overview"
              hidden={tab !== "overview"}
              className={`flex-1 min-h-0 flex flex-col justify-between overflow-y-auto ${tab !== "overview" ? "hidden" : ""}`}
            >
              <div className="stat-grid performance-stats shrink-0 m-0 grid grid-cols-2 border-y border-glass-divider bg-surface/25 min-[621px]:grid-cols-3 min-[1281px]:grid-cols-6 [&>.stat-card]:border-r [&>.stat-card]:border-glass-divider [&>.stat-card]:px-3.5 [&>.stat-card]:py-2.5 [&>.stat-card:last-child]:border-r-0 max-[1280px]:[&>.stat-card:nth-child(n+4)]:border-t max-[620px]:[&>.stat-card:nth-child(2n)]:border-r-0 min-[621px]:max-[1280px]:[&>.stat-card:nth-child(3n)]:border-r-0">
                {cards.map((card) => (
                  <div className="stat-card flex min-w-0 flex-col hover:bg-glass-hover" key={card.label}>
                    <span className="stat-label text-[10.5px] font-semibold uppercase tracking-wider text-muted">{card.label}</span>
                    <div className="stat-main mt-0.5 mb-px flex items-end justify-between gap-1.5">
                      <strong className="stat-value min-w-0 text-xl font-bold leading-tight tracking-tight tabular-nums">{card.value}</strong>
                      {summary?.requests ? <Sparkline values={card.spark} tone={card.tone} /> : null}
                    </div>
                    <span className="stat-hint truncate text-[11px] leading-snug text-faint">{card.hint}</span>
                  </div>
                ))}
              </div>
              <div className="performance-trends flex-1 min-h-0 grid grid-cols-1 gap-3.5 px-4.5 py-3 min-[1001px]:grid-cols-2 max-[1000px]:p-2 items-stretch">
                <PerformanceTrend series={data.series} kind="rate" />
                <PerformanceTrend series={data.series} kind="latency" />
              </div>
              <div className={`performance-insights shrink-0 grid grid-cols-1 border-t border-glass-divider ${quality ? "min-[1001px]:grid-cols-2" : "min-[1001px]:grid-cols-1 single"} [&>section]:min-w-0 [&>section]:px-4.5 [&>section]:pt-3.5 [&>section]:pb-4 [&>section+section]:border-glass-divider min-[1001px]:[&>section+section]:border-l max-[1000px]:[&>section+section]:border-t [&_h3]:mb-3 [&_h3]:flex [&_h3]:flex-wrap [&_h3]:items-baseline [&_h3]:justify-between [&_h3]:gap-x-2.5 [&_h3]:text-xs [&_h3]:font-semibold [&_h3_span]:text-[10.5px] [&_h3_span]:font-medium [&_h3_span]:text-faint`}>
                <section>
                  <h3>
                    Services by errors <span>Top 6 · request errors in the window</span>
                  </h3>
                  <ServiceErrorBars services={data.services} from={from} to={to} />
                  {data.endpoints.length > 0 && (
                    <button
                      type="button"
                      onClick={() => switchTab("endpoints")}
                      className="mt-3.5 pt-2 border-t border-black/5 dark:border-white/10 text-xs font-semibold text-sky-600 dark:text-sky-400 hover:text-sky-700 dark:hover:text-sky-300 inline-flex items-center gap-1.5 transition-colors cursor-pointer"
                    >
                      <span>View all {data.endpoints.length} endpoints breakdown</span>
                      <span aria-hidden="true">&rarr;</span>
                    </button>
                  )}
                </section>
                {quality && (
                  <section>
                    <h3>
                      Telemetry quality <span>Can logs and spans be followed?</span>
                    </h3>
                    <QualityDonut quality={quality} />
                  </section>
                )}
              </div>
            </div>

            {/* Endpoints Tab Panel: Dedicated breakdown table */}
            <div
              role="tabpanel"
              id="panel-endpoints"
              aria-labelledby="tab-endpoints"
              hidden={tab !== "endpoints"}
              className={`flex flex-1 min-h-0 flex-col overflow-hidden ${tab !== "endpoints" ? "hidden" : ""}`}
            >
              <div className="performance-breakdown flex flex-1 min-h-0 flex-col overflow-hidden border-t border-glass-divider">
                <div className="performance-breakdown-head shrink-0 flex flex-wrap items-center justify-between gap-3 border-b border-glass-divider bg-surface/20 px-4.5 py-3 [&_h3]:m-0 [&_h3]:flex [&_h3]:flex-wrap [&_h3]:items-baseline [&_h3]:gap-2.5 [&_h3]:text-xs [&_h3_span]:text-[10.5px] [&_h3_span]:text-faint">
                  <button
                    type="button"
                    onClick={() => switchTab("overview")}
                    className={`performance-back-btn ${secondaryButtonClass}`}
                  >
                    <ArrowLeft size={14} aria-hidden="true" />
                    <span>Back to Overview</span>
                  </button>
                  <h3>
                    By endpoint <span>Top {data.limit} · templated HTTP route, RPC method, or span name</span>
                  </h3>
                </div>
                <DataTable
                  headers={["Service", "Endpoint", ...headers]}
                  empty="No endpoints observed in this window."
                  rows={data.endpoints.map((row) => [
                    serviceLink(row.service, from, to),
                    row.endpoint,
                    ...columns(row),
                  ])}
                />
              </div>
            </div>
          </div>
        )
      )}
    </Panel>
  );
}
