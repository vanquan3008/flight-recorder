import { useQueries } from "@tanstack/react-query";
import { Activity, AlertTriangle, ArrowRight, BellRing, Database, Gauge, RadioTower, ShieldCheck, Sparkles } from "lucide-react";
import { Link } from "react-router-dom";
import { DetailLink } from "../../shared/navigation";
import { useAuth } from "../../app/auth";
import { api, queryString } from "../../shared/api";
import { TimeWindowFilter, useTimeWindow } from "../../shared/time-window";
import type { Incident, Page, TelemetryQuality, TraceSummary } from "../../shared/types";
import { Badge, DataTable, ErrorState, Loading, PageHeader, Panel, formatDuration, formatTime } from "../../shared/ui";
import { Onboarding } from "../onboarding/onboarding";
import { PerformanceDashboard } from "../performance/dashboard";

const latest = <T,>(path: string, apiKey: string) => ({
  queryKey: ["overview", path],
  queryFn: ({ signal }: { signal: AbortSignal }) => api<Page<T>>(path, apiKey, { signal }),
});

export function OverviewPage() {
  const { apiKey } = useAuth();
  const window = useTimeWindow();
  const filters = queryString({ from: window.from, to: window.to });
  const qualityPath = `/api/v1/telemetry/quality?${filters}`;
  const [incidents, quality, traces] = useQueries({
    queries: [
      latest<Incident>(`/api/v1/incidents?${filters}`, apiKey),
      { queryKey: ["overview", qualityPath], queryFn: ({ signal }: { signal: AbortSignal }) => api<TelemetryQuality>(qualityPath, apiKey, { signal }) },
      latest<TraceSummary>(`/api/v1/traces?${filters}`, apiKey),
    ],
  });
  const queries = [incidents, quality, traces];
  if (queries.some((query) => query.isPending)) return <Loading />;
  const failed = queries.find((query) => query.isError);
  if (failed) return <ErrorState error={failed.error} retry={() => queries.forEach((query) => void query.refetch())} />;

  const loadedIncidents = incidents.data?.data || [];
  const loadedTraces = traces.data?.data || [];
  const actionable = loadedIncidents.filter((incident) => incident.status === "OPEN" || incident.status === "INVESTIGATING");
  const cards = [
    { label: "Incidents needing action", value: actionable.length, path: `/incidents?${filters}`, hint: `open or investigating · ${loadedIncidents.length} incidents on this page (max 50)`, Icon: BellRing, color: "text-rose-400 bg-rose-500/10" },
    { label: "Traces", value: loadedTraces.length, path: `/traces?${filters}`, hint: "latest page in window · max 50", Icon: Activity, color: "text-sky-400 bg-sky-500/10" },
    { label: "Failed traces", value: loadedTraces.filter((trace) => trace.error_count > 0).length, path: `/traces?${filters}&status=ERROR`, hint: "with an error span · latest page only", Icon: AlertTriangle, color: "text-amber-400 bg-amber-500/10" },
    { label: "Metric points", value: quality.data?.metric_points || 0, hint: "all stored points in window", Icon: Gauge, color: "text-emerald-400 bg-emerald-500/10" },
  ];
  return <>
    <PageHeader title="Telemetry overview">Start with a failed trace, inspect its spans, then follow the correlated logs.</PageHeader>
    <div className="overview-hero rounded-3xl backdrop-blur-2xl bg-gradient-to-br from-sky-50 via-blue-50/60 to-white dark:from-slate-900/80 dark:via-blue-950/40 dark:to-slate-900/80 border border-sky-200/80 dark:border-white/10 p-7 md:p-9 shadow-[0_12px_40px_rgba(37,99,235,0.08)] relative overflow-hidden mb-6 flex flex-col md:flex-row items-start md:items-center justify-between gap-6">
      <div className="overview-hero-body max-w-2xl relative z-10">
        <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full backdrop-blur-md bg-sky-500/15 dark:bg-sky-400/15 border border-sky-500/30 text-sky-800 dark:text-sky-300 text-[10.5px] font-black uppercase tracking-widest mb-3">Unified Telemetry Platform</span>
        <h2 className="text-2xl md:text-3xl font-black tracking-tight text-slate-950 dark:text-white m-0 mb-2 leading-tight">Work is always better when systems are observable</h2>
        <p className="text-[13.5px] text-slate-600 dark:text-slate-300 m-0 mb-5 leading-relaxed font-medium">Real-time OpenTelemetry trace streams, correlated logs, and automated incident diagnosis.</p>
        <div className="overview-hero-pills flex flex-wrap items-center gap-2.5">
          <Link to={`/traces?${filters}`} className="inline-flex items-center gap-2 px-5 py-2.5 rounded-full bg-gradient-to-r from-sky-500 via-sky-600 to-blue-600 text-white font-bold text-xs shadow-md shadow-sky-500/35 hover:scale-[1.02] active:scale-95 transition-all no-underline">Explore traces →</Link>
          <Link to={`/incidents?${filters}`} className="inline-flex items-center gap-2 px-5 py-2.5 rounded-full backdrop-blur-md bg-white/80 dark:bg-sky-500/10 border border-sky-300 dark:border-sky-500/30 text-sky-800 dark:text-sky-300 font-bold text-xs hover:bg-sky-50 dark:hover:bg-sky-500/20 active:scale-95 transition-all no-underline">Active incidents ({actionable.length})</Link>
        </div>
      </div>
      <div className="overview-hero-graphic relative flex items-center justify-center w-36 h-32 shrink-0 self-center" aria-hidden="true">
        <div className="w-28 h-28 rounded-full bg-gradient-to-tr from-blue-500 to-cyan-400 blur-2xl opacity-60 animate-pulse" />
        <div className="absolute inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-full backdrop-blur-xl bg-white/95 dark:bg-slate-900/90 text-slate-900 dark:text-white text-xs font-bold border border-white/60 dark:border-white/10 shadow-lg"><span className="text-emerald-500">● Live</span> 99.9% health</div>
      </div>
    </div>
    <TimeWindowFilter range={window.range} select={window.select} />
    <Onboarding apiKey={apiKey} hasTrace={Boolean(traces.data?.data.length)} />
    <div className="stat-grid grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">{cards.map((card) => {
      const { Icon, color } = card;
      const content = <>
        <div className="flex items-center justify-between mb-2">
          <span className="text-[11px] font-black uppercase tracking-wider text-slate-600 dark:text-slate-300">{card.label}</span>
          <span className={`p-2 rounded-xl ${color}`}><Icon size={18} /></span>
        </div>
        <strong className="block text-3xl font-black tracking-tight text-slate-950 dark:text-white my-1">{card.value}</strong>
        <span className="text-xs text-slate-600 dark:text-slate-400 font-medium">{card.hint}</span>
      </>;
      const cardClasses = "stat-card flex flex-col justify-between p-5 rounded-2xl backdrop-blur-xl bg-white/85 dark:bg-slate-900/75 border border-slate-200/80 dark:border-white/10 shadow-[0_6px_24px_rgba(31,38,135,0.06)] hover:border-sky-400/60 hover:-translate-y-0.5 hover:shadow-lg transition-all no-underline text-inherit";
      return card.path ? <Link className={cardClasses} to={card.path} key={card.label}>{content}</Link> : <div className={cardClasses} key={card.label}>{content}</div>;
    })}</div>

    <Panel title="Telemetry Architecture & Data Flow" meta="Real-time OpenTelemetry Pipeline">
      <div className="grid grid-cols-1 lg:grid-cols-[1fr_auto_1fr_auto_1fr_auto_1fr_auto_1fr] items-center gap-2 p-6">
        <div className="flex flex-col items-center text-center p-4 rounded-2xl backdrop-blur-md bg-white/80 dark:bg-slate-900/60 border border-slate-200/80 dark:border-white/10 shadow-xs hover:border-blue-400 hover:-translate-y-0.5 transition-all">
          <div className="inline-flex items-center justify-center w-10 h-10 rounded-xl bg-blue-500/15 text-blue-500 mb-2"><Activity size={20} /></div>
          <strong className="text-xs font-bold text-slate-900 dark:text-slate-100">Your Apps & SDKs</strong>
          <small className="text-[11px] text-slate-600 dark:text-slate-400 font-medium mt-0.5">OpenTelemetry Traces & Logs</small>
        </div>
        <div className="text-blue-500/60 flex items-center justify-center rotate-90 lg:rotate-0"><ArrowRight size={18} /></div>
        <div className="flex flex-col items-center text-center p-4 rounded-2xl backdrop-blur-md bg-white/80 dark:bg-slate-900/60 border border-slate-200/80 dark:border-white/10 shadow-xs hover:border-blue-400 hover:-translate-y-0.5 transition-all">
          <div className="inline-flex items-center justify-center w-10 h-10 rounded-xl bg-sky-500/15 text-sky-500 mb-2"><RadioTower size={20} /></div>
          <strong className="text-xs font-bold text-slate-900 dark:text-slate-100">OTLP Receiver</strong>
          <small className="text-[11px] text-slate-600 dark:text-slate-400 font-medium mt-0.5">gRPC :4317 · HTTP :4318</small>
        </div>
        <div className="text-blue-500/60 flex items-center justify-center rotate-90 lg:rotate-0"><ArrowRight size={18} /></div>
        <div className="flex flex-col items-center text-center p-4 rounded-2xl backdrop-blur-md bg-white/80 dark:bg-slate-900/60 border border-slate-200/80 dark:border-white/10 shadow-xs hover:border-blue-400 hover:-translate-y-0.5 transition-all">
          <div className="inline-flex items-center justify-center w-10 h-10 rounded-xl bg-indigo-500/15 text-indigo-500 mb-2"><ShieldCheck size={20} /></div>
          <strong className="text-xs font-bold text-slate-900 dark:text-slate-100">Pipeline Queue</strong>
          <small className="text-[11px] text-slate-600 dark:text-slate-400 font-medium mt-0.5">Byte budget & Redaction</small>
        </div>
        <div className="text-blue-500/60 flex items-center justify-center rotate-90 lg:rotate-0"><ArrowRight size={18} /></div>
        <div className="flex flex-col items-center text-center p-4 rounded-2xl backdrop-blur-md bg-white/80 dark:bg-slate-900/60 border border-slate-200/80 dark:border-white/10 shadow-xs hover:border-blue-400 hover:-translate-y-0.5 transition-all">
          <div className="inline-flex items-center justify-center w-10 h-10 rounded-xl bg-emerald-500/15 text-emerald-500 mb-2"><Database size={20} /></div>
          <strong className="text-xs font-bold text-slate-900 dark:text-slate-100">PostgreSQL 17</strong>
          <small className="text-[11px] text-slate-600 dark:text-slate-400 font-medium mt-0.5">Partitioned commit ACK</small>
        </div>
        <div className="text-blue-500/60 flex items-center justify-center rotate-90 lg:rotate-0"><ArrowRight size={18} /></div>
        <div className="flex flex-col items-center text-center p-4 rounded-2xl backdrop-blur-md bg-white/80 dark:bg-slate-900/60 border border-slate-200/80 dark:border-white/10 shadow-xs hover:border-blue-400 hover:-translate-y-0.5 transition-all">
          <div className="inline-flex items-center justify-center w-10 h-10 rounded-xl bg-purple-500/15 text-purple-500 mb-2"><Sparkles size={20} /></div>
          <strong className="text-xs font-bold text-slate-900 dark:text-slate-100">RCA & Console</strong>
          <small className="text-[11px] text-slate-600 dark:text-slate-400 font-medium mt-0.5">Instant diagnosis & Waterfall</small>
        </div>
      </div>
      <div className="px-6 pb-4 pt-1 border-t border-black/5 dark:border-white/10">
        <Link to="/flow" className="inline-flex items-center gap-1.5 text-xs font-bold text-blue-600 dark:text-blue-400 hover:underline">Learn more about architectural flow & guarantees →</Link>
      </div>
    </Panel>

    {actionable.length > 0 && <Panel title="Incidents needing action" meta={`${actionable.length} on this page`}><DataTable
      headers={["Last seen", "Severity", "Status", "Service", "Title", ""]}
      rows={actionable.slice(0, 5).map((incident) => [formatTime(incident.last_seen_at), <Badge value={incident.severity} />, <Badge value={incident.status} />, incident.primary_service, incident.title, <DetailLink to={`/incidents/${incident.id}`}>Investigate</DetailLink>])}
    /></Panel>}
    <PerformanceDashboard from={window.from} to={window.to} />
    <Panel title="Recent traces" meta="latest 10"><DataTable
      headers={["Started", "Service", "Duration", "Spans", "Status", ""]}
      empty="No traces yet. Complete the setup steps above to send the first one."
      rows={(traces.data?.data || []).slice(0, 10).map((trace) => [
        formatTime(trace.started_at), trace.root_service, formatDuration(trace.duration_nanos), trace.span_count, <Badge value={trace.error_count ? "ERROR" : "OK"} />, <DetailLink to={`/traces/${trace.trace_id}`}>Inspect</DetailLink>,
      ])}
    /></Panel>
  </>;
}
