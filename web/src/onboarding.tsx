import { useState } from "react";
import { Link } from "react-router-dom";
import { Panel } from "./ui";

const otlpConfig = `OTEL_SERVICE_NAME=my-service
OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
OTEL_EXPORTER_OTLP_ENDPOINT=http://<tracepilot-host>:4318
OTEL_EXPORTER_OTLP_HEADERS=x-api-key=<ingest-api-key>`;

export function Onboarding({ hasTrace }: { hasTrace: boolean }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    await navigator.clipboard.writeText(otlpConfig);
    setCopied(true);
  };
  return <Panel title="Get your first trace" meta={hasTrace ? "Complete" : "3 steps"}>
    <div className="onboarding"><p>TracePilot is ready when a real trace appears below. Use an ingest-scoped key for your application, not the console key.</p>
      <div className="onboarding-steps">
        <section className="onboarding-step complete"><strong>1. Connect</strong><p>Your console key is connected for this browser session.</p></section>
        <section className="onboarding-step"><strong>2. Configure OTLP</strong><p>Set these variables for one OpenTelemetry SDK, then replace both placeholders.</p><code>{otlpConfig}</code><button type="button" className="button-secondary" onClick={() => void copy()}>{copied ? "Copied" : "Copy OTLP config"}</button></section>
        <section className={`onboarding-step ${hasTrace ? "complete" : ""}`}><strong>3. Confirm signal</strong>{hasTrace ? <p>First trace received. <Link to="/traces">Inspect traces</Link>.</p> : <p>Send a request from your instrumented application, then refresh this page.</p>}</section>
      </div>
    </div>
  </Panel>;
}
