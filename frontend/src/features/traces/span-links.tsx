import { ArrowUpRight } from "lucide-react";
import { useLocation } from "react-router-dom";
import { DetailLink } from "../../shared/ui";
import { contextURL, traceURL } from "../../shared/ui/navigation";
import { KeyValueTable } from "./span-kv-table";

export type SpanLink = { traceID: string; spanID: string; traceState: string; attributes: Record<string, unknown> };

const text = (value: unknown) => (typeof value === "string" ? value : "");

/** Reads span links in the API's Go field names, accepting snake and camel case too. */
export function spanLinks(raw: unknown): SpanLink[] {
  if (!Array.isArray(raw)) return [];
  return raw.flatMap((item) => {
    if (!item || typeof item !== "object") return [];
    const link = item as Record<string, unknown>;
    const traceID = text(link.TraceID ?? link.trace_id ?? link.traceId);
    if (!traceID) return [];
    const attributes = link.Attributes ?? link.attributes;
    return [{
      traceID, spanID: text(link.SpanID ?? link.span_id ?? link.spanId), traceState: text(link.TraceState ?? link.trace_state ?? link.traceState),
      attributes: attributes && typeof attributes === "object" && !Array.isArray(attributes) ? attributes as Record<string, unknown> : {},
    }];
  });
}

export function SpanLinks({ links }: { links: SpanLink[] }) {
  const location = useLocation();
  return <div className="divide-y divide-line-soft">{links.map((link, index) => <article key={`${link.traceID}:${link.spanID}:${index}`} className="px-4 py-3 wrap-anywhere">
    <DetailLink to={contextURL(traceURL(link.traceID, link.spanID), location.search, true)} className="inline-flex items-center gap-1 text-xs font-medium text-accent no-underline hover:underline">
      Open linked trace<ArrowUpRight size={12} />
    </DetailLink>
    <p className="m-0 mt-1 font-mono text-[11px] text-muted">Trace {link.traceID}</p>
    {link.spanID && <p className="m-0 mt-0.5 font-mono text-[11px] text-muted">Span {link.spanID}</p>}
    {link.traceState && <p className="m-0 mt-0.5 font-mono text-[11px] text-muted">tracestate {link.traceState}</p>}
    {Object.keys(link.attributes).length > 0 && <div className="-mx-4 mt-2"><KeyValueTable data={link.attributes} /></div>}
  </article>)}</div>;
}
