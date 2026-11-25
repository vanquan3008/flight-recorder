import { secondaryButtonClass } from "../../shared/ui/styles";
import { Activity, ArrowRight, BellRing, CheckCircle2, Database, FileSearch, GitBranch, KeyRound, RadioTower, Search, ShieldCheck, Sparkles } from "lucide-react";
import type { ComponentType } from "react";
import { Fragment } from "react";
import { Link } from "react-router-dom";

type FlowNode = {
  icon: ComponentType<{ size?: number }>;
  step: string;
  title: string;
  copy: string;
  tags: string[];
};

const writeFlow: FlowNode[] = [
  { icon: Activity, step: "01 · Producer", title: "Your applications", copy: "OpenTelemetry SDKs emit application traces, logs, metrics and errors.", tags: ["OTel SDK", "OTLP"] },
  { icon: RadioTower, step: "02 · Write API", title: "OTLP ingestion API", copy: "A dedicated gRPC or HTTP listener accepts standard OpenTelemetry export requests.", tags: ["gRPC :4317", "HTTP :4318", "scope: ingest"] },
  { icon: ShieldCheck, step: "03 · Admission", title: "Validate and normalize", copy: "Authentication, size limits and redaction run before records enter the bounded pipeline.", tags: ["Backpressure", "Canonical event"] },
  { icon: Database, step: "04 · Durability", title: "Persist telemetry", copy: "TracePilot acknowledges traces and logs after they flush to ClickHouse and the PostgreSQL incident transaction for any failures commits.", tags: ["ClickHouse", "PostgreSQL", "Commit before ACK"] },
];

const readFlow: FlowNode[] = [
  { icon: Search, step: "01 · Consumer", title: "Console or API client", copy: "The browser console and external tools request telemetry independently of ingestion.", tags: ["React Console", "External client"] },
  { icon: FileSearch, step: "02 · Read API", title: "Management REST API", copy: "A separate HTTP listener exposes versioned query and management endpoints.", tags: [":8080/api/v1", "JSON", "scope: read"] },
  { icon: KeyRound, step: "03 · Authorization", title: "Project-scoped query", copy: "The signed-in account and its role in the chosen project decide what it can read; a project you do not belong to answers as forbidden.", tags: ["JWT access token", "Project roles"] },
  { icon: Database, step: "04 · Result", title: "Read stored telemetry", copy: "Scoped queries read logs, traces and metrics from ClickHouse and incidents from PostgreSQL.", tags: ["Read path", "JSON response"] },
];

const intelligenceFlow: FlowNode[] = [
  { icon: GitBranch, step: "01 · Relate", title: "Service/dependency graph", copy: "Parent-child spans and semantic attributes reveal service, database, queue and external edges.", tags: ["Topology", "Trace evidence"] },
  { icon: Activity, step: "02 · Detect", title: "Baseline deviation", copy: "A five-minute window is compared with the preceding thirty minutes for latency, errors, throughput and backlog.", tags: ["Current vs baseline", "Async worker"] },
  { icon: BellRing, step: "03 · Investigate", title: "Incident and RCA", copy: "Ranked hypotheses and supported dependency correlations point back to stored evidence and shared trace IDs.", tags: ["Hypothesis", "Evidence IDs"] },
];

const firstRun = [
  [KeyRound, "Sign in", "Use the account the super administrator created, then replace its temporary password."],
  [Sparkles, "Create a project", "Make a project for your product, then an ingest token for it."],
  [RadioTower, "Send a signal", "Copy the OTLP variables into one application."],
  [Search, "Find the cause", "Open a failed trace and inspect its linked logs."],
] as const;

export function FlowPage() {
  return <div className="flow-page [--flow-violet:var(--violet)] [--flow-cyan:var(--cyan)] [--flow-edge:color-mix(in_srgb,var(--blue)_35%,var(--line))]">
    <section className="flow-hero border border-glass-edge bg-glass backdrop-glass shadow-glass relative mb-5.5 grid min-h-[280px] grid-cols-[minmax(0,1fr)_auto] items-end gap-8 overflow-hidden rounded-3xl p-9.5 max-[900px]:min-h-0 max-[900px]:grid-cols-1 max-[900px]:items-start max-[900px]:p-7.5 max-[620px]:px-5 max-[620px]:py-6 [&_h1]:mb-3.5 [&_h1]:max-w-[980px] [&_h1]:text-[clamp(36px,4.2vw,56px)] [&_h1]:leading-[1.05] [&_h1]:tracking-tighter [&_h1_span]:bg-linear-to-r [&_h1_span]:from-accent [&_h1_span]:via-cyan [&_h1_span]:to-violet [&_h1_span]:bg-clip-text [&_h1_span]:text-transparent [&_p]:m-0 [&_p]:max-w-[920px] [&_p]:text-[15px] [&_p]:leading-relaxed [&_p]:text-muted">
      <div>
        <span className="flow-eyebrow mb-4.5 inline-flex items-center gap-1.75 rounded-full border border-accent/35 bg-accent/10 px-2.5 py-1.5 text-[11px] font-bold tracking-wider text-accent uppercase"><Sparkles size={14} /> How TracePilot works</span>
        <h1>From signal to answer,<br /><span>without losing the thread.</span></h1>
        <p>TracePilot exposes two independent API contracts: producers write telemetry through OTLP, while the Console and external clients read it through the Management API.</p>
      </div>
      <div className="flow-actions grid min-w-[190px] gap-2.25 max-[900px]:flex max-[900px]:min-w-0 max-[900px]:flex-wrap max-[620px]:w-full">
        <Link className={`flow-button ${secondaryButtonClass} min-h-10.5 px-3.5 py-2.25 no-underline transition-transform hover:-translate-y-px max-[620px]:w-full border-btn-border bg-btn text-btn-text primary`} to="/overview">Start setup <ArrowRight size={16} /></Link>
        <Link className={`flow-button ${secondaryButtonClass} min-h-10.5 px-3.5 py-2.25 no-underline transition-transform hover:-translate-y-px max-[620px]:w-full`} to="/traces">Open trace explorer</Link>
      </div>
    </section>

    <section className="flow-contracts mb-5.5 grid grid-cols-2 gap-3 max-[900px]:grid-cols-1 [&>article]:grid [&>article]:min-w-0 [&>article]:grid-cols-[auto_minmax(0,1fr)_auto] [&>article]:items-center [&>article]:gap-3.5 [&>article]:rounded-xl [&>article]:border [&>article]:border-glass-divider [&>article]:bg-glass-nested [&>article]:p-4.5 [&>article>svg]:text-cyan [&_article_span]:text-[10px] [&_article_span]:font-bold [&_article_span]:tracking-wider [&_article_span]:text-accent [&_article_span]:uppercase [&_h2]:mt-0.5 [&_h2]:mb-0.75 [&_h2]:text-lg [&_p]:m-0 [&_p]:text-xs [&_p]:text-muted [&_code]:rounded-lg [&_code]:border [&_code]:border-line [&_code]:bg-surface-sunken [&_code]:px-2.25 [&_code]:py-1.5 [&_code]:whitespace-nowrap [&>p]:col-span-full [&>p]:flex [&>p]:items-center [&>p]:justify-center [&>p]:gap-1.75 [&>p]:text-center [&>p]:text-faint max-[620px]:[&>article]:grid-cols-[auto_minmax(0,1fr)] max-[620px]:[&_code]:col-span-full max-[620px]:[&_code]:text-center" aria-label="API boundaries">
      <article className="write"><RadioTower size={22} /><div><span>Write boundary</span><h2>OTLP ingestion</h2><p>Applications push telemetry into TracePilot.</p></div><code>:4317 · :4318</code></article>
      <article className="read"><FileSearch size={22} /><div><span>Read boundary</span><h2>Management REST</h2><p>Console and clients query telemetry from TracePilot.</p></div><code>:8080/api/v1</code></article>
      <p><Database size={16} /> ClickHouse stores logs, traces and metrics; PostgreSQL stores incidents and project settings.</p>
    </section>

    <FlowLane eyebrow="Write API · Ingestion" title="Applications push OpenTelemetry data into TracePilot" nodes={writeFlow} />
    <FlowLane eyebrow="Read API · Management" title="People and tools query committed data from TracePilot" nodes={readFlow} tone="read" />
    <FlowLane eyebrow="Observability intelligence" title="Telemetry → graph → incident → evidence-backed RCA" nodes={intelligenceFlow} tone="optional" />

    <section className="flow-first-run border border-glass-edge bg-glass backdrop-glass shadow-glass mb-5.5 rounded-2xl p-6 max-[620px]:px-3.75 max-[620px]:py-4.75 [&>header]:mb-5 [&>header>span]:text-[10px] [&>header>span]:font-bold [&>header>span]:tracking-wider [&>header>span]:text-accent [&>header>span]:uppercase [&_h2]:mt-1.25 [&_h2]:text-xl" aria-labelledby="first-run-title">
      <header><span>First 10 minutes</span><h2 id="first-run-title">Your path to the first useful trace</h2></header>
      <div className="flow-steps grid grid-cols-4 gap-2.5 max-[1200px]:grid-cols-2 max-[620px]:grid-cols-1 [&>article]:grid [&>article]:min-w-0 [&>article]:grid-cols-[auto_auto_minmax(0,1fr)] [&>article]:items-start [&>article]:gap-2.5 [&>article]:rounded-xl [&>article]:border [&>article]:border-glass-divider [&>article]:bg-glass-nested [&>article]:p-3.75 [&_svg]:mt-0.5 [&_svg]:text-blue [&_strong]:mb-0.75 [&_strong]:block [&_p]:m-0 [&_p]:text-xs [&_p]:text-muted">{firstRun.map(([Icon, title, copy], index) => <article key={title}>
        <span className="flow-step-number grid size-5.5 place-items-center rounded-md bg-neutral-bg text-[10px] font-bold text-neutral-fg">{index + 1}</span><Icon size={21} /><div><strong>{title}</strong><p>{copy}</p></div>
      </article>)}</div>
      <div className="flow-promise mt-3.5 flex items-start gap-2.25 rounded-lg border border-accent/40 bg-accent/10 px-3.75 py-3.25 text-xs text-text [&>svg]:shrink-0 [&>svg]:text-accent"><CheckCircle2 size={19} /><span><strong>The durability promise:</strong> traces and logs are acknowledged after ClickHouse flushes them and the PostgreSQL incident transaction for any failures commits. Metrics remain best-effort by design.</span></div>
    </section>
  </div>;
}

function FlowLane({ eyebrow, title, nodes, tone = "" }: { eyebrow: string; title: string; nodes: FlowNode[]; tone?: "read" | "optional" | "" }) {
  const laneText = tone === "read" ? "text-violet" : tone === "optional" ? "text-danger" : "text-accent";
  const laneHoverBorder = tone === "read" ? "hover:border-violet" : tone === "optional" ? "hover:border-danger" : "hover:border-accent";
  const iconBox = tone === "read" ? "border-violet/30 bg-violet/13 text-violet" : tone === "optional" ? "border-danger/30 bg-danger/13 text-danger" : "border-accent/30 bg-accent/13 text-accent";

  return <section className={`flow-lane border border-glass-edge bg-glass backdrop-glass shadow-glass mb-5.5 rounded-2xl p-6 max-[620px]:px-3.75 max-[620px]:py-4.75 [&>header]:mb-5 [&>header>span]:text-[10px] [&>header>span]:font-bold [&>header>span]:tracking-wider [&>header>span]:uppercase [&_h2]:mt-1.25 [&_h2]:text-xl [&_h2]:tracking-tight ${tone}`} aria-label={eyebrow}>
    <header><span className={laneText}>{eyebrow}</span><h2>{title}</h2></header>
    <div className="flow-chain grid grid-cols-1 items-stretch min-[1201px]:grid-cols-[minmax(0,1fr)_42px_minmax(0,1fr)_42px_minmax(0,1fr)_42px_minmax(0,1fr)]">{nodes.map((node, index) => <Fragment key={node.title}>
      <article className={`flow-node relative min-h-[238px] min-w-0 overflow-hidden rounded-xl border border-glass-divider bg-glass-nested p-4.5 transition-transform hover:-translate-y-0.75 ${laneHoverBorder} max-[1200px]:min-h-0 [&>small]:mb-1 [&>small]:block [&>small]:text-[10px] [&>small]:font-bold [&>small]:tracking-wider [&>small]:uppercase [&>h3]:mb-1.75 [&>h3]:text-[17px] [&>p]:mb-4 [&>p]:text-xs [&>p]:text-muted`}>
        <div className={`flow-node-icon mb-6.75 grid size-10.75 place-items-center rounded-xl border max-[1200px]:mb-4.5 ${iconBox}`}><node.icon size={22} /></div>
        <small className={laneText}>{node.step}</small><h3>{node.title}</h3><p>{node.copy}</p>
        <div className="flow-tags relative z-1 flex flex-wrap gap-1.5 [&>span]:rounded-full [&>span]:border [&>span]:border-line [&>span]:bg-surface-sunken [&>span]:px-1.75 [&>span]:py-1 [&>span]:text-[10px] [&>span]:font-bold [&>span]:text-muted">{node.tags.map((tag) => <span key={tag}>{tag}</span>)}</div>
      </article>
      {index < nodes.length - 1 && <span className="flow-arrow grid place-items-center text-faint max-[1200px]:min-h-9.5 max-[1200px]:rotate-90 [&>svg]:rounded-full [&>svg]:border [&>svg]:border-line [&>svg]:bg-surface-raised [&>svg]:p-1" aria-hidden="true"><ArrowRight size={20} /></span>}
    </Fragment>)}</div>
  </section>;
}
