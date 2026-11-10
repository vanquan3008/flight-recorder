import { useQuery } from "@tanstack/react-query";
import { ArrowDownRight, ArrowUpRight, ChevronDown, ChevronLeft, ChevronRight, FileSearch, GitBranch, Search, X } from "lucide-react";
import { useMemo, useRef, useState } from "react";
import { useAuth } from "../../app/auth";
import { api } from "../../shared/api";
import type { IncidentAnalysis, IncidentEvidence } from "../../shared/types";
import { CopyButton, DetailLink, Empty, ErrorState, Loading } from "../../shared/ui";
import { secondaryButtonClass } from "../../shared/ui/styles";

const number = (value: number) => Number.isFinite(value) ? value.toLocaleString("en", { maximumSignificantDigits: 7 }) : "—";
const score = (value: number) => Number.isFinite(value) ? value.toFixed(2) : "—";
const cardClass = "overflow-hidden rounded-2xl border border-line bg-surface-solid shadow-sm";

export function IncidentAnalysisPanels({ incidentID }: { incidentID: string }) {
  const { credential } = useAuth();
  const query = useQuery({ queryKey: ["incident-analysis", incidentID], queryFn: ({ signal }) => api<IncidentAnalysis>(`/api/v1/incidents/${incidentID}/analysis`, credential, { signal }) });
  const [selectedIDs, setSelectedIDs] = useState<string[]>([]);
  const [search, setSearch] = useState("");
  const [evidencePage, setEvidencePage] = useState(0);
  const [hypothesisPage, setHypothesisPage] = useState(0);
  const [relationshipPage, setRelationshipPage] = useState(0);
  const evidenceRef = useRef<HTMLElement>(null);
  const selected = useMemo(() => new Set(selectedIDs), [selectedIDs]);
  const evidence = query.data?.evidence ?? [];
  const filtered = useMemo(() => {
    const text = search.trim().toLowerCase();
    return evidence.filter((item) => (!selected.size || selected.has(item.id)) && (!text || `${item.id} ${item.metric_name} ${item.entity_id} ${item.series_id ?? ""} ${item.summary}`.toLowerCase().includes(text)));
  }, [evidence, selected, search]);
  if (query.isPending) return <Loading />;
  if (query.isError) return <ErrorState error={query.error} retry={() => void query.refetch()} />;
  const analysis = query.data;
  const hp = Math.min(hypothesisPage, Math.max(0, Math.ceil(analysis.hypotheses.length / 3) - 1));
  const ep = Math.min(evidencePage, Math.max(0, Math.ceil(filtered.length / 8) - 1));
  const rp = Math.min(relationshipPage, Math.max(0, Math.ceil(analysis.causal_graph.length / 8) - 1));
  const missing = selectedIDs.filter((id) => !evidence.some((item) => item.id === id));
  return <div className="grid min-w-0 grid-cols-1 items-start gap-4 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)]">
    <section className={cardClass} aria-labelledby="hypotheses-title">
      <header className="flex flex-wrap items-center justify-between gap-2 border-b border-line-soft px-5 py-4"><h2 id="hypotheses-title" className="m-0 inline-flex items-center gap-2 text-sm font-semibold"><FileSearch size={16} className="text-accent" />RCA hypotheses</h2><span className="text-[11px] text-muted">{analysis.hypotheses.length} ranked</span></header>
      <p className="m-0 border-b border-line-soft bg-surface-sunken/30 px-5 py-3 text-xs leading-relaxed text-muted">Hypotheses are ranked by heuristic score. Scores are not probabilities or proof of the root cause.</p>
      {analysis.hypotheses.length ? <ol className="m-0 list-none divide-y divide-line-soft p-0">{analysis.hypotheses.slice(hp * 3, hp * 3 + 3).map((item) => <li key={`${item.rank}:${item.entity_id}`} className="flex items-start gap-3 p-5">
        <span className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-accent/10 font-mono text-xs font-semibold text-accent">#{item.rank}</span>
        <div className="min-w-0 flex-1">
          <div className="mb-2 flex flex-wrap items-center justify-between gap-2"><strong className="break-words text-[13px] font-semibold">{item.entity_id}</strong><span className="whitespace-nowrap rounded-full bg-neutral-bg px-2 py-1 font-mono text-[10px] text-muted">Heuristic score {score(item.confidence)}</span></div>
          <p className="m-0 break-words text-[13px] leading-relaxed text-muted">{item.statement}</p>
          {item.evidence_ids.length > 0 && <details className="mt-2 text-[11px] text-faint"><summary className="cursor-pointer">Evidence references</summary><p className="mt-2 break-all font-mono text-[10px]">{item.evidence_ids.join(", ")}</p></details>}
          {item.evidence_ids.length > 0 && <button type="button" className="mt-3 inline-flex items-center gap-1 text-xs font-medium text-accent hover:underline" onClick={() => {
            setSelectedIDs(item.evidence_ids); setSearch(""); setEvidencePage(0); evidenceRef.current?.scrollIntoView?.({ block: "start" });
          }}>View supporting evidence ({item.evidence_ids.length})<ArrowDownRight size={13} /></button>}
        </div>
      </li>)}</ol> : <Empty>Not enough evidence has been captured to rank a hypothesis.</Empty>}
      <CollectionPager page={hp} size={3} count={analysis.hypotheses.length} onChange={setHypothesisPage} label="hypotheses" />
      {analysis.conclusion && <details className="border-t border-line-soft px-5 py-3"><summary className="cursor-pointer text-xs font-medium text-muted">Full analysis summary</summary><p className="mt-3 break-words text-xs leading-relaxed text-muted">{analysis.conclusion}</p></details>}
    </section>

    <section ref={evidenceRef} className={`${cardClass} scroll-mt-4`} aria-labelledby="evidence-title">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line-soft px-5 py-4">
        <div><h2 id="evidence-title" className="m-0 text-sm font-semibold">Evidence</h2><p className="mt-1 text-[11px] text-muted">{filtered.length} of {evidence.length} returned signals{selected.size ? " · filtered by hypothesis" : ""}</p></div>
        <label className="relative min-w-0"><Search size={13} className="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2 text-faint" /><input type="search" aria-label="Search evidence" placeholder="Metric, service or evidence ID" value={search} onChange={(event) => { setSearch(event.target.value); setEvidencePage(0); }} className="h-8 w-full rounded-lg border border-line bg-glass-field pr-3 pl-8 text-xs sm:w-60" /></label>
      </header>
      {selected.size > 0 && <div className="flex items-center justify-between gap-2 border-b border-line-soft bg-accent/5 px-5 py-2 text-xs text-muted"><span>Supporting signals{missing.length ? ` · ${missing.length} referenced records are not in this response` : ""}</span><button type="button" className="inline-flex shrink-0 items-center gap-1 text-accent" onClick={() => { setSelectedIDs([]); setEvidencePage(0); }}>Clear selection<X size={12} /></button></div>}
      {filtered.length ? <div className="grid grid-cols-[repeat(auto-fit,minmax(min(280px,100%),1fr))] gap-3 p-4">{filtered.slice(ep * 8, ep * 8 + 8).map((item) => <EvidenceCard key={item.id} item={item} />)}</div>
        : <Empty>{evidence.length ? "No returned evidence matches this selection." : "No evidence records are stored for this incident."}</Empty>}
      <CollectionPager page={ep} size={8} count={filtered.length} onChange={setEvidencePage} label="signals" />
    </section>

    <details className={`${cardClass} group xl:col-span-full`}>
      <summary className="flex cursor-pointer list-none items-center justify-between gap-3 px-5 py-4 text-sm font-semibold"><span className="inline-flex items-center gap-2"><GitBranch size={15} className="text-muted" />Evidence relationships</span><span className="inline-flex items-center gap-2 text-xs font-normal text-muted">{analysis.causal_graph.length} supported<ChevronDown size={14} className="transition-transform group-open:rotate-180" /></span></summary>
      {!analysis.causal_graph.length ? <p className="m-0 border-t border-line-soft px-5 py-4 text-xs leading-relaxed text-muted">No supported relationships were found. Dependency evidence, a shared trace and overlapping signal windows are needed to relate signals.</p>
        : <ul className="m-0 max-h-80 list-none divide-y divide-line-soft overflow-y-auto border-t border-line-soft p-0">{analysis.causal_graph.slice(rp * 8, rp * 8 + 8).map((edge, index) => <li key={index} className="px-5 py-3 text-xs"><p className="m-0 break-words font-medium">{edge.source_entity_id} → {edge.target_entity_id}</p><p className="mt-1 text-muted">{edge.relationship} · heuristic score {score(edge.confidence)}</p>{edge.evidence_ids.length > 0 && <details className="mt-2"><summary className="cursor-pointer text-faint">Evidence references ({edge.evidence_ids.length})</summary><p className="mt-2 break-all font-mono text-[10px] text-muted">{edge.evidence_ids.join(", ")}</p></details>}</li>)}</ul>}
      <CollectionPager page={rp} size={8} count={analysis.causal_graph.length} onChange={setRelationshipPage} label="relationships" />
    </details>
  </div>;
}

function EvidenceCard({ item }: { item: IncidentEvidence }) {
  const factor = item.baseline_value > 0 && item.observed_value >= 0 ? item.observed_value / item.baseline_value : undefined;
  return <article className="evidence-card min-w-0 rounded-xl border border-line-soft bg-surface-sunken/30 p-4">
    <div className="mb-2 flex flex-wrap items-center justify-between gap-2"><span className="rounded bg-neutral-bg px-1.5 py-0.5 text-[9px] font-semibold tracking-wide text-muted uppercase">{item.signal_type}</span><span className="inline-flex items-center gap-1 text-[10px] text-warn-fg">{item.direction === "increase" ? <ArrowUpRight size={12} /> : item.direction === "decrease" ? <ArrowDownRight size={12} /> : null}{item.direction}</span></div>
    <h3 className="m-0 break-words font-mono text-xs font-semibold">{item.metric_name}</h3><p className="mt-1 break-words text-[11px] text-muted">{item.entity_id}</p>
    <dl className="my-3 grid grid-cols-2 gap-3 rounded-lg border border-line-soft bg-surface-solid p-3"><div><dt className="mb-1 text-[10px] text-muted">Observed</dt><dd className="m-0 break-all font-mono text-sm font-semibold" title={String(item.observed_value)}>{number(item.observed_value)}</dd></div><div><dt className="mb-1 text-[10px] text-muted">Baseline</dt><dd className="m-0 break-all font-mono text-sm" title={String(item.baseline_value)}>{number(item.baseline_value)}</dd></div></dl>
    {factor !== undefined && Number.isFinite(factor) && <p className="mb-3 text-[11px] text-muted"><strong className="font-semibold text-text">{factor.toLocaleString("en", { maximumSignificantDigits: 3 })}×</strong> baseline</p>}
    <details className="text-[11px] text-muted"><summary className="cursor-pointer font-medium">Signal details &amp; trace links</summary><div className="mt-3 space-y-2">
      <p className="break-words leading-relaxed">{item.summary}</p>
      <div className="flex items-center justify-between gap-2"><span className="break-all font-mono text-[10px]">ID: {item.id}</span><CopyButton value={item.id} label="Copy evidence ID" /></div>
      {item.series_id && <p className="break-all font-mono text-[10px]">Series: {item.series_id}</p>}
      <p className="break-words text-[10px]">Entity type: {item.entity_type} · Reported change ratio: {number(item.change_ratio)}</p>
      <p className="break-words font-mono text-[10px]">Raw values: {String(item.observed_value)} / {String(item.baseline_value)}</p>
      {item.trace_ids.length ? <div className="flex flex-wrap gap-2">{item.trace_ids.map((traceID) => <DetailLink key={traceID} to={`/traces/${traceID}`} className="break-all font-mono text-[10px] text-accent hover:underline">Trace {traceID} ↗</DetailLink>)}</div> : <p>No linked traces for this signal.</p>}
    </div></details>
  </article>;
}

function CollectionPager({ page, size, count, onChange, label }: { page: number; size: number; count: number; onChange: (page: number) => void; label: string }) {
  if (count <= size) return null;
  return <div className="flex flex-wrap items-center justify-between gap-2 border-t border-line-soft px-5 py-3 text-[11px] text-muted"><span>{page * size + 1}–{Math.min((page + 1) * size, count)} of {count} {label}</span><div className="flex gap-2"><button type="button" aria-label={`Previous ${label}`} className={secondaryButtonClass} disabled={!page} onClick={() => onChange(page - 1)}><ChevronLeft size={12} />Previous</button><button type="button" aria-label={`Next ${label}`} className={secondaryButtonClass} disabled={(page + 1) * size >= count} onClick={() => onChange(page + 1)}>Next<ChevronRight size={12} /></button></div></div>;
}
