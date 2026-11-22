import { primaryButtonClass, secondaryButtonClass } from "../../shared/ui/styles";
import { Check } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { api, APIError } from "../../shared/api";
import { Panel } from "../../shared/ui";

const otlpConfig = (ingestToken: string, environment: string) => `OTEL_SERVICE_NAME=my-service
OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
OTEL_EXPORTER_OTLP_ENDPOINT=http://<tracepilot-host>:4318
OTEL_EXPORTER_OTLP_HEADERS=authorization=Bearer%20${ingestToken || "<ingest-token>"}${environment ? `,x-environment=${encodeURIComponent(environment)}` : ""}`;

type IssuedIngestToken = { jwt: string };

/** `canCreate` is false for a user who was only added to the project: its owner makes the ingest tokens. */
export function Onboarding({ credential, projectID, environment, canCreate, hasTrace }: { credential: string; projectID: string; environment: string; canCreate: boolean; hasTrace: boolean }) {
  const [ingestKey, setIngestKey] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState("");
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);
  const configuration = otlpConfig(ingestKey, environment);
  const createIngestToken = async () => {
    setCreating(true);
    setCreateError("");
    try {
      const issued = await api<IssuedIngestToken>(`/api/v1/projects/${projectID}/ingest-tokens`, credential, {
        method: "POST",
        body: JSON.stringify({ name: `first-app-${crypto.randomUUID().slice(0, 8)}` }),
      });
      setIngestKey(issued.jwt);
    } catch (error) {
      setCreateError(error instanceof APIError && error.status === 403
        ? "Only an owner of this project can create an ingest token."
        : error instanceof Error ? error.message : "Unable to create an ingest token.");
    } finally {
      setCreating(false);
    }
  };
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(configuration);
      setCopied(true);
      setCopyFailed(false);
    } catch {
      setCopyFailed(true);
    }
  };
  if (hasTrace) return <section className="onboarding-done mb-3.5 flex flex-wrap items-baseline gap-x-3 gap-y-1 px-1 [&_h2]:m-0 [&_h2]:text-[10px] [&_h2]:font-bold [&_h2]:tracking-widest [&_h2]:text-faint [&_h2]:uppercase [&_p]:m-0 [&_p]:text-[12.5px] [&_p]:text-muted [&_a]:text-accent [&_a]:no-underline [&_a:hover]:underline"><h2>Get your first trace</h2><p>First trace received. <Link to="/traces">Inspect traces</Link>.</p></section>;
  // Steps run in order; the first one not yet done is the current one.
  const done = [true, Boolean(ingestKey), copied, false];
  const current = done.indexOf(false);
  const steps = [
    { title: "Sign in", body: <p>You are signed in to this project.</p> },
    { title: "Create ingest token", body: <>
      {ingestKey ? <><p>The token is shown once. Copy the configuration before leaving this page.</p><code>{ingestKey}</code></>
        : <>{canCreate ? <><p>Create a token that can only send telemetry into this project, for your first application.</p><button type="button" className={`step-action ${primaryButtonClass}`} disabled={creating} onClick={() => void createIngestToken()}>{creating ? "Creating…" : "Create ingest token"}</button></> : <p>Ask an owner of this project to create an ingest token for your application.</p>}</>}
      {createError && <p className="form-error mt-1.5 text-[12.5px] font-medium leading-relaxed text-danger-fg" role="alert">{createError}</p>}
    </> },
    { title: "Configure OTLP", body: <>
      <p>Set these variables for one OpenTelemetry SDK. Replace the host placeholder for remote deployments.</p>
      <code>{configuration}</code>
      <button type="button" className={`button-secondary ${secondaryButtonClass}`} disabled={!ingestKey} onClick={() => void copy()}>{copied ? "Copied" : "Copy OTLP config"}</button>
      {copyFailed && <p className="form-error mt-1.5 text-[12.5px] font-medium leading-relaxed text-danger-fg" role="status">Clipboard access was denied. Select the template and copy it manually.</p>}
    </> },
    { title: "Confirm signal", body: <p>Send a request from your instrumented application, then refresh this page.</p> },
  ];
  return <Panel title="Get your first trace" meta={`${done.filter(Boolean).length} of ${steps.length} done`}>
    <div className="onboarding grid px-4 pt-1 pb-4.5">
      <p className="onboarding-lead mt-2 mb-4 text-[12.5px] text-muted">TracePilot is ready when a real trace appears below. Keep your sign-in out of your application: it uses the ingest token.</p>
      <ol className="stepper m-0 grid max-w-[760px] list-none p-0 [&>li]:relative [&>li]:grid [&>li]:grid-cols-[24px_minmax(0,1fr)] [&>li]:gap-3.5 [&>li]:pb-4.5 [&>li:last-child]:pb-0 [&>li:not(:last-child)]:before:absolute [&>li:not(:last-child)]:before:top-7 [&>li:not(:last-child)]:before:bottom-1 [&>li:not(:last-child)]:before:left-2.75 [&>li:not(:last-child)]:before:w-0.5 [&>li:not(:last-child)]:before:bg-glass-divider [&>li.done]:before:bg-svc-1/55">{steps.map((step, index) => <li key={step.title} className={done[index] ? "done" : index === current ? "current" : ""} aria-current={index === current ? "step" : undefined}>
        <span className="step-marker grid size-6 place-items-center rounded-full border border-glass-field-edge bg-glass-field text-[11.5px] font-bold text-faint in-[.current]:border-accent in-[.current]:text-accent in-[.done]:border-transparent in-[.done]:bg-svc-1 in-[.done]:text-bg" aria-hidden="true">{done[index] ? <Check size={13} strokeWidth={3} /> : index + 1}</span>
        <div className="step-body grid min-w-0 justify-items-start gap-1.5 pt-0.5 [&>strong]:text-[13px] [&>strong]:font-semibold [&_p]:m-0 [&_p]:text-[12.5px] [&_p]:text-muted [&_code]:justify-self-stretch [&_code]:rounded-lg [&_code]:border [&_code]:border-glass-divider [&_code]:bg-glass-sunken [&_code]:px-3 [&_code]:py-2.5 [&_code]:text-[11.5px] [&_code]:leading-relaxed [&_code]:whitespace-pre-wrap [&_code]:wrap-anywhere"><strong>{step.title}</strong>{step.body}</div>
      </li>)}</ol>
    </div>
  </Panel>;
}
