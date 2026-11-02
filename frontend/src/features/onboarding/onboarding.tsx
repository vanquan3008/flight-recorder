import { Check } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { api, APIError } from "../../shared/api";
import { Panel } from "../../shared/ui";

const otlpConfig = (ingestToken = "<ingest-token>") => `OTEL_SERVICE_NAME=my-service
OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
OTEL_EXPORTER_OTLP_ENDPOINT=http://<tracepilot-host>:4318
OTEL_EXPORTER_OTLP_HEADERS=authorization=Bearer%20${ingestToken}`;

type IssuedIngestToken = { jwt: string };

/** `canCreate` is false for a user who was only added to the project: its owner makes the ingest tokens. */
export function Onboarding({ credential, projectID, canCreate, hasTrace }: { credential: string; projectID: string; canCreate: boolean; hasTrace: boolean }) {
  const [ingestKey, setIngestKey] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState("");
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);
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
      await navigator.clipboard.writeText(otlpConfig(ingestKey));
      setCopied(true);
      setCopyFailed(false);
    } catch {
      setCopyFailed(true);
    }
  };
  if (hasTrace) return <section className="onboarding-done"><h2>Get your first trace</h2><p>First trace received. <Link to="/traces">Inspect traces</Link>.</p></section>;
  // Steps run in order; the first one not yet done is the current one.
  const done = [true, Boolean(ingestKey), copied, false];
  const current = done.indexOf(false);
  const steps = [
    { title: "Sign in", body: <p>You are signed in to this project.</p> },
    { title: "Create ingest token", body: <>
      {ingestKey ? <><p>The token is shown once. Copy the configuration before leaving this page.</p><code>{ingestKey}</code></>
        : <>{canCreate ? <><p>Create a token that can only send telemetry into this project, for your first application.</p><button type="button" className="step-action" disabled={creating} onClick={() => void createIngestToken()}>{creating ? "Creating…" : "Create ingest token"}</button></> : <p>Ask an owner of this project to create an ingest token for your application.</p>}</>}
      {createError && <p className="form-error" role="alert">{createError}</p>}
    </> },
    { title: "Configure OTLP", body: <>
      <p>Set these variables for one OpenTelemetry SDK. Replace the host placeholder for remote deployments.</p>
      <code>{otlpConfig(ingestKey)}</code>
      <button type="button" className="button-secondary" disabled={!ingestKey} onClick={() => void copy()}>{copied ? "Copied" : "Copy OTLP config"}</button>
      {copyFailed && <p className="form-error" role="status">Clipboard access was denied. Select the template and copy it manually.</p>}
    </> },
    { title: "Confirm signal", body: <p>Send a request from your instrumented application, then refresh this page.</p> },
  ];
  return <Panel title="Get your first trace" meta={`${done.filter(Boolean).length} of ${steps.length} done`}>
    <div className="onboarding">
      <p className="onboarding-lead">TracePilot is ready when a real trace appears below. Keep your sign-in out of your application: it uses the ingest token.</p>
      <ol className="stepper">{steps.map((step, index) => <li key={step.title} className={done[index] ? "done" : index === current ? "current" : ""} aria-current={index === current ? "step" : undefined}>
        <span className="step-marker" aria-hidden="true">{done[index] ? <Check size={13} strokeWidth={3} /> : index + 1}</span>
        <div className="step-body"><strong>{step.title}</strong>{step.body}</div>
      </li>)}</ol>
    </div>
  </Panel>;
}
