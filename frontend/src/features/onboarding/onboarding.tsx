import { useState } from "react";
import { Link } from "react-router-dom";
import { api, APIError } from "../../shared/api";
import { Panel } from "../../shared/ui";

const otlpConfig = (ingestKey = "<ingest-api-key>") => `OTEL_SERVICE_NAME=my-service
OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
OTEL_EXPORTER_OTLP_ENDPOINT=http://<tracepilot-host>:4318
OTEL_EXPORTER_OTLP_HEADERS=x-api-key=${ingestKey}`;

type IssuedAPIKey = { key: string };

export function Onboarding({ apiKey, hasTrace }: { apiKey: string; hasTrace: boolean }) {
  const [ingestKey, setIngestKey] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState("");
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);
  const createIngestKey = async () => {
    setCreating(true);
    setCreateError("");
    try {
      const issued = await api<IssuedAPIKey>("/api/v1/api-keys", apiKey, {
        method: "POST",
        body: JSON.stringify({ name: `first-app-${crypto.randomUUID().slice(0, 8)}`, scopes: ["ingest"] }),
      });
      setIngestKey(issued.key);
    } catch (error) {
      setCreateError(error instanceof APIError && error.status === 403
        ? "Connect with the bootstrap or an admin-scoped key to create an ingestion key."
        : error instanceof Error ? error.message : "Unable to create an ingestion key.");
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
  if (hasTrace) return <Panel title="Get your first trace" meta="Complete"><div className="onboarding"><p>First trace received. <Link to="/traces">Inspect traces</Link>.</p></div></Panel>;
  return <Panel title="Get your first trace" meta="4 steps">
    <div className="onboarding">
      <p>TracePilot is ready when a real trace appears below. Keep the console key out of your application.</p>
      <div className="onboarding-steps">
        <section className="onboarding-step complete"><strong>1. Connect</strong><p>Your console key is connected for this browser session.</p></section>
        <section className={`onboarding-step ${ingestKey ? "complete" : ""}`}>
          <strong>2. Create ingestion key</strong>
          {ingestKey ? <><p>The key is shown once. Copy the configuration before leaving this page.</p><code>{ingestKey}</code></> : <><p>Create a least-privilege key for your first application.</p><button type="button" disabled={creating} onClick={() => void createIngestKey()}>{creating ? "Creating…" : "Create ingestion key"}</button></>}
          {createError && <p className="form-error" role="alert">{createError}</p>}
        </section>
        <section className="onboarding-step">
          <strong>3. Configure OTLP</strong>
          <p>Set these variables for one OpenTelemetry SDK. Replace the host placeholder for remote deployments.</p>
          <code>{otlpConfig(ingestKey)}</code>
          <button type="button" className="button-secondary" disabled={!ingestKey} onClick={() => void copy()}>{copied ? "Copied" : "Copy OTLP config"}</button>
          {copyFailed && <p className="form-error" role="status">Clipboard access was denied. Select the template and copy it manually.</p>}
        </section>
        <section className="onboarding-step">
          <strong>4. Confirm signal</strong>
          <p>Send a request from your instrumented application, then refresh this page.</p>
        </section>
      </div>
    </div>
  </Panel>;
}
