import { describe, expect, it } from "vitest";
import { analyzeTraceInsights, formatShortUrl, getSpanDisplayInfo } from "./trace-format";
import { buildTrace } from "./trace-model";
import type { Span } from "../../shared/types";

describe("formatShortUrl", () => {
  it("shortens long URLs cleanly", () => {
    const url = "https://s3-sgn10.fptcloud.com/kaiva/kalo/staging/user-avatar/01a17d055aa77ed3af4036130a927eba/avatar.png";
    expect(formatShortUrl(url)).toBe("s3-sgn10.fptcloud.com/.../avatar.png");
  });

  it("leaves short URLs readable", () => {
    const url = "https://example.com/api/v1/users";
    expect(formatShortUrl(url)).toBe("example.com/api/v1/users");
  });
});

describe("getSpanDisplayInfo", () => {
  it("enhances bare GET span using url.full", () => {
    const span: Span = {
      trace_id: "t1",
      span_id: "s1",
      name: "GET",
      service_name: "kalo-service",
      started_at: "2026-11-04T00:00:00Z",
      ended_at: "2026-11-04T00:00:01Z",
      status_code: "ERROR",
      attributes: {
        "url.full": "https://s3-sgn10.fptcloud.com/kaiva/kalo/staging/avatar.png",
        "http.response.status_code": 503,
      },
      resource_attributes: {},
      events: [],
      links: [],
    };
    const info = getSpanDisplayInfo(span);
    expect(info.method).toBe("GET");
    expect(info.displayName).toBe("GET s3-sgn10.fptcloud.com/kaiva/kalo/staging/avatar.png");
    expect(info.httpStatus).toBe(503);
    expect(info.isClientCall).toBe(true);
  });

  it("preserves already informative span names", () => {
    const span: Span = {
      trace_id: "t1",
      span_id: "s2",
      name: "GET /api/orders/123",
      service_name: "gateway",
      started_at: "2026-11-04T00:00:00Z",
      ended_at: "2026-11-04T00:00:01Z",
      status_code: "OK",
      attributes: {},
      resource_attributes: {},
      events: [],
      links: [],
    };
    const info = getSpanDisplayInfo(span);
    expect(info.displayName).toBe("GET /api/orders/123");
    expect(info.method).toBe("GET");
  });
});

describe("analyzeTraceInsights", () => {
  it("detects retry storm on same client target", () => {
    const base = Date.parse("2026-11-04T00:00:00Z");
    const spans: Span[] = [
      {
        trace_id: "t1",
        span_id: "root",
        name: "GET /avatar",
        service_name: "kalo",
        started_at: new Date(base).toISOString(),
        ended_at: new Date(base + 38000).toISOString(),
        status_code: "UNSET",
        attributes: {},
        resource_attributes: {},
        events: [],
        links: [],
      },
      ...Array.from({ length: 5 }, (_, i) => ({
        trace_id: "t1",
        span_id: `child-${i}`,
        parent_span_id: "root",
        name: "GET",
        service_name: "kalo",
        started_at: new Date(base + i * 2000).toISOString(),
        ended_at: new Date(base + i * 2000 + 50).toISOString(),
        status_code: "ERROR",
        attributes: {
          "url.full": "https://s3-sgn10.fptcloud.com/file.png",
          "http.response.status_code": 503,
        },
        resource_attributes: {},
        events: [],
        links: [],
      })),
    ];
    const model = buildTrace(spans);
    const insights = analyzeTraceInsights(model);
    expect(insights.length).toBeGreaterThan(0);
    expect(insights.some((ins) => ins.title.includes("retry failure pattern"))).toBe(true);
  });
});
