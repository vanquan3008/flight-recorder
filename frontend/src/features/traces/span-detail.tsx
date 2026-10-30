import { DetailLink } from "../../shared/navigation";
import type { Span } from "../../shared/types";
import { CopyButton, json } from "../../shared/ui";
import { formatMs } from "./trace-model";

type Props = { span: Span; traceID: string; startMs: number; durationMs: number };

export function SpanDetail({ span, traceID, startMs, durationMs }: Props) {
  return <section className="span-detail" aria-label={`Span ${span.name}`}>
    <div className="detail-heading">
      <div>
        <h3>{span.name}</h3>
        <p>{span.service_name} · {span.status_code || "UNSET"} · starts at +{formatMs(startMs)} · {formatMs(durationMs)}</p>
        <DetailLink to={`/logs?traceId=${traceID}&spanId=${span.span_id}`}>Logs for this span</DetailLink>
      </div>
      <CopyButton value={span.span_id} label="Copy span ID" />
    </div>
    {[["Attributes", span.attributes], ["Resource", span.resource_attributes], ["Events", span.events], ["Links", span.links]].map(([name, value]) =>
      <div key={String(name)}><h4>{String(name)}</h4><pre>{json(value)}</pre></div>)}
  </section>;
}
