import {
  AlertOctagon,
  AlertTriangle,
  CheckCircle,
  Clock,
  Code2,
  FileText,
  Globe,
  Layers,
  Server,
  Sliders,
  X,
} from "lucide-react";
import { useState } from "react";
import type { Span } from "../../shared/types";
import { CopyButton, DetailLink, json } from "../../shared/ui";
import { KeyValueTable, findException } from "./span-kv-table";
import { getSpanDisplayInfo } from "./trace-format";
import { formatMs, isError } from "./trace-model";

type Props = {
  span: Span;
  traceID: string;
  startMs: number;
  durationMs: number;
  onClose?: () => void;
};

export function SpanDetail({ span, traceID, startMs, durationMs, onClose }: Props) {
  const info = getSpanDisplayInfo(span);
  const failed = isError(span);
  const attrs = (span.attributes as Record<string, unknown>) || {};
  const statusDisplay = span.status_code || "UNSET";
  const exception = findException(span);
  const [activeTab, setActiveTab] = useState<"attributes" | "resource" | "events" | "raw">("attributes");

  const fullUrl = String(attrs["url.full"] || info.target || "");
  const hostPeer = attrs["server.address"]
    ? `${String(attrs["server.address"])}${attrs["server.port"] ? `:${String(attrs["server.port"])}` : ""}`
    : "";

  const attrCount = Object.keys(attrs).length;
  const resCount = Object.keys((span.resource_attributes as Record<string, unknown>) || {}).length;
  const events = (span.events as Array<Record<string, unknown>>) || [];
  const eventCount = events.length;

  return (
    <section className="span-detail" aria-label={`Span ${span.name}`}>
      {/* Sleek Top Heading Bar */}
      <div className="detail-heading">
        <div className="detail-title-group">
          <div className="detail-meta-bar">
            <span className="detail-svc-pill">
              <span className="svc-dot" />
              {span.service_name}
            </span>
            <span className={`detail-status-pill ${failed ? "error" : statusDisplay === "OK" ? "success" : "neutral"}`}>
              {failed ? <AlertOctagon size={13} aria-hidden="true" /> : <CheckCircle size={13} aria-hidden="true" />}
              {info.httpStatus ? `HTTP ${info.httpStatus} · ` : ""}
              {statusDisplay}
            </span>
            {info.method && <span className="detail-method-pill">{info.method}</span>}
            <span className="detail-metric-pill">
              <Clock size={12} />
              {formatMs(durationMs)}
            </span>
          </div>
          <h3 className="detail-title">{span.name}</h3>
          <p className="detail-subtitle">
            <strong>{span.service_name}</strong> · {statusDisplay} · starts at +{formatMs(startMs)} · {formatMs(durationMs)}
          </p>
        </div>

        <div className="detail-header-controls">
          <DetailLink to={`/logs?traceId=${traceID}&spanId=${span.span_id}`} className="detail-logs-btn">
            <FileText size={13} /> Logs for this span
          </DetailLink>
          <CopyButton value={span.span_id} label="Copy span ID" />
          {onClose && (
            <button type="button" className="button-secondary compact detail-close-btn" aria-label="Close span details" onClick={onClose}>
              <X size={15} aria-hidden="true" />
            </button>
          )}
        </div>
      </div>

      {/* Prominent Exception Alert Banner */}
      {exception && (
        <div className="span-exception-banner">
          <div className="exception-header">
            <div className="exception-badge">
              <AlertTriangle size={15} />
              <span>{exception.type}</span>
            </div>
            {exception.timestamp && <span className="exception-time">{exception.timestamp}</span>}
          </div>
          <p className="exception-msg">{exception.message}</p>
          {exception.stacktrace && (
            <details className="exception-stacktrace">
              <summary>View Stack Trace</summary>
              <div className="stacktrace-body">
                <div className="stacktrace-copy">
                  <CopyButton value={exception.stacktrace} label="Copy stack trace" />
                </div>
                <pre>{exception.stacktrace}</pre>
              </div>
            </details>
          )}
        </div>
      )}

      {/* Modern Bento Highlights Grid */}
      {(fullUrl || hostPeer || Boolean(attrs["error.type"])) && (
        <div className="detail-quick-grid">
          {Boolean(fullUrl) && (
            <div className="bento-item">
              <div className="bento-icon-box blue"><Globe size={14} /></div>
              <div className="bento-content">
                <span className="bento-label">Target / Endpoint</span>
                <span className="bento-value mono" title={fullUrl}>{fullUrl}</span>
              </div>
              <CopyButton value={fullUrl} label="Copy URL" />
            </div>
          )}
          {Boolean(hostPeer) && (
            <div className="bento-item">
              <div className="bento-icon-box purple"><Server size={14} /></div>
              <div className="bento-content">
                <span className="bento-label">Host / Peer</span>
                <span className="bento-value mono">{hostPeer}</span>
              </div>
              <CopyButton value={hostPeer} label="Copy host" />
            </div>
          )}
          {Boolean(attrs["error.type"]) && (
            <div className="bento-item bento-error">
              <div className="bento-icon-box red"><AlertOctagon size={14} /></div>
              <div className="bento-content">
                <span className="bento-label">Error Type</span>
                <span className="bento-value error-text mono">{String(attrs["error.type"])}</span>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Tab Navigation for Detailed Properties */}
      <div className="detail-tabs-nav" role="tablist">
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === "attributes"}
          className={`detail-tab-btn${activeTab === "attributes" ? " active" : ""}`}
          onClick={() => setActiveTab("attributes")}
        >
          <Sliders size={13} />
          Attributes
          <span className="tab-badge">{attrCount}</span>
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === "resource"}
          className={`detail-tab-btn${activeTab === "resource" ? " active" : ""}`}
          onClick={() => setActiveTab("resource")}
        >
          <Layers size={13} />
          Resource
          <span className="tab-badge">{resCount}</span>
        </button>
        {eventCount > 0 && (
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === "events"}
            className={`detail-tab-btn${activeTab === "events" ? " active" : ""}`}
            onClick={() => setActiveTab("events")}
          >
            <Clock size={13} />
            Events
            <span className="tab-badge">{eventCount}</span>
          </button>
        )}
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === "raw"}
          className={`detail-tab-btn${activeTab === "raw" ? " active" : ""}`}
          onClick={() => setActiveTab("raw")}
        >
          <Code2 size={13} />
          Raw JSON
        </button>
      </div>

      {/* Tab Content Panels */}
      <div className="detail-tab-content">
        {activeTab === "attributes" && (
          <KeyValueTable data={attrs} placeholder="Filter span attributes..." />
        )}
        {activeTab === "resource" && (
          <KeyValueTable data={span.resource_attributes as Record<string, unknown>} placeholder="Filter resource attributes..." />
        )}
        {activeTab === "events" && (
          <div className="events-timeline">
            {events.map((ev, i) => (
              <div key={i} className="event-card">
                <div className="event-head">
                  <span className="event-name">{String(ev.name || ev.Name || `Event #${i + 1}`)}</span>
                  <span className="event-time">{String(ev.timestamp || ev.Timestamp || "")}</span>
                </div>
                {Boolean(ev.attributes || ev.Attributes) && (
                  <KeyValueTable data={(ev.attributes || ev.Attributes) as Record<string, unknown>} />
                )}
              </div>
            ))}
          </div>
        )}
        {activeTab === "raw" && (
          <div className="raw-json-wrap">
            <div className="raw-json-toolbar">
              <span>Full Span JSON</span>
              <CopyButton value={json(span)} label="Copy full payload" />
            </div>
            <pre>{json(span)}</pre>
          </div>
        )}
      </div>
    </section>
  );
}
