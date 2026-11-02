import { Activity, ArrowRight, BellRing, CheckCircle2, Database, FileSearch, GitBranch, KeyRound, RadioTower, Search, ShieldCheck, Sparkles } from "lucide-react";
import type { ComponentType } from "react";
import { Fragment } from "react";
import { Link } from "react-router-dom";
import "./flow.css";

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
  { icon: Database, step: "04 · Durability", title: "Commit to PostgreSQL", copy: "TracePilot acknowledges traces and logs only after their database transaction commits.", tags: ["Transaction", "Commit before ACK"] },
];

const readFlow: FlowNode[] = [
  { icon: Search, step: "01 · Consumer", title: "Console or API client", copy: "The browser console and external tools request telemetry independently of ingestion.", tags: ["React Console", "External client"] },
  { icon: FileSearch, step: "02 · Read API", title: "Management REST API", copy: "A separate HTTP listener exposes versioned query and management endpoints.", tags: [":8080/api/v1", "JSON", "scope: read"] },
  { icon: KeyRound, step: "03 · Authorization", title: "Project-scoped query", copy: "The signed-in account and its role in the chosen project decide what it can read; a project you do not belong to answers as forbidden.", tags: ["JWT access token", "Project roles"] },
  { icon: Database, step: "04 · Result", title: "Read from PostgreSQL", copy: "Scoped queries return traces, logs and incidents without entering the ingestion pipeline.", tags: ["Read path", "JSON response"] },
];

const intelligenceFlow: FlowNode[] = [
  { icon: GitBranch, step: "01 · Relate", title: "Service/dependency graph", copy: "Parent-child spans and semantic attributes reveal service, database, queue and external edges.", tags: ["Topology", "Trace evidence"] },
  { icon: Activity, step: "02 · Detect", title: "Baseline deviation", copy: "A five-minute window is compared with the preceding thirty minutes for latency, errors, throughput and backlog.", tags: ["Current vs baseline", "Async worker"] },
  { icon: BellRing, step: "03 · Investigate", title: "Incident and RCA", copy: "Ranked hypotheses and supported dependency correlations point back to stored evidence and shared trace IDs.", tags: ["Hypothesis", "Evidence IDs"] },
];

const firstRun = [
  [KeyRound, "Register", "Create an account; the first one becomes the super administrator."],
  [Sparkles, "Create a project", "Make a project for your product, then an ingest token for it."],
  [RadioTower, "Send a signal", "Copy the OTLP variables into one application."],
  [Search, "Find the cause", "Open a failed trace and inspect its linked logs."],
] as const;

export function FlowPage() {
  return <div className="flow-page">
    <section className="flow-hero">
      <div>
        <span className="flow-eyebrow"><Sparkles size={14} /> How TracePilot works</span>
        <h1>From signal to answer,<br /><span>without losing the thread.</span></h1>
        <p>TracePilot exposes two independent API contracts: producers write telemetry through OTLP, while the Console and external clients read it through the Management API.</p>
      </div>
      <div className="flow-actions">
        <Link className="flow-button primary" to="/overview">Start setup <ArrowRight size={16} /></Link>
        <Link className="flow-button" to="/traces">Open trace explorer</Link>
      </div>
    </section>

    <section className="flow-contracts" aria-label="API boundaries">
      <article className="write"><RadioTower size={22} /><div><span>Write boundary</span><h2>OTLP ingestion</h2><p>Applications push telemetry into TracePilot.</p></div><code>:4317 · :4318</code></article>
      <article className="read"><FileSearch size={22} /><div><span>Read boundary</span><h2>Management REST</h2><p>Console and clients query telemetry from TracePilot.</p></div><code>:8080/api/v1</code></article>
      <p><Database size={16} /> The APIs share PostgreSQL as storage, not request handlers or transport contracts.</p>
    </section>

    <FlowLane eyebrow="Write API · Ingestion" title="Applications push OpenTelemetry data into TracePilot" nodes={writeFlow} />
    <FlowLane eyebrow="Read API · Management" title="People and tools query committed data from TracePilot" nodes={readFlow} tone="read" />
    <FlowLane eyebrow="Observability intelligence" title="Telemetry → graph → incident → evidence-backed RCA" nodes={intelligenceFlow} tone="optional" />

    <section className="flow-first-run" aria-labelledby="first-run-title">
      <header><span>First 10 minutes</span><h2 id="first-run-title">Your path to the first useful trace</h2></header>
      <div className="flow-steps">{firstRun.map(([Icon, title, copy], index) => <article key={title}>
        <span className="flow-step-number">{index + 1}</span><Icon size={21} /><div><strong>{title}</strong><p>{copy}</p></div>
      </article>)}</div>
      <div className="flow-promise"><CheckCircle2 size={19} /><span><strong>The durability promise:</strong> traces and logs are acknowledged only after PostgreSQL commits. Metrics remain best-effort by design.</span></div>
    </section>
  </div>;
}

function FlowLane({ eyebrow, title, nodes, tone = "" }: { eyebrow: string; title: string; nodes: FlowNode[]; tone?: "read" | "optional" | "" }) {
  return <section className={`flow-lane ${tone}`} aria-label={eyebrow}>
    <header><span>{eyebrow}</span><h2>{title}</h2></header>
    <div className="flow-chain">{nodes.map((node, index) => <Fragment key={node.title}>
      <article className="flow-node">
        <div className="flow-node-icon"><node.icon size={22} /></div>
        <small>{node.step}</small><h3>{node.title}</h3><p>{node.copy}</p>
        <div className="flow-tags">{node.tags.map((tag) => <span key={tag}>{tag}</span>)}</div>
      </article>
      {index < nodes.length - 1 && <span className="flow-arrow" aria-hidden="true"><ArrowRight size={20} /></span>}
    </Fragment>)}</div>
  </section>;
}
