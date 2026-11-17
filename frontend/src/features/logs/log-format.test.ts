import { describe, expect, it } from "vitest";
import { normalizeSeverityText, parseHttpLog } from "./log-format";

describe("parseHttpLog", () => {
  it("parses inbound HTTP access log with headers and remote IP", () => {
    const raw = `HTTP Inbound "GET" "/api/v1/businesses/01a1a0f4-dca1-7e41-9d00-f00c0c715e6d/chats"?page=1" returned 200 in 280.2ms
Remote IP: "::1"
Request Headers: [{"Accept": "*/*"}, {"Connection": "close"}, {"Host": "localhost:7119"}]`;

    const result = parseHttpLog(raw);
    expect(result).not.toBeNull();
    expect(result?.isHttp).toBe(true);
    expect(result?.direction).toBe("Inbound");
    expect(result?.method).toBe("GET");
    expect(result?.path).toBe("/api/v1/businesses/01a1a0f4-dca1-7e41-9d00-f00c0c715e6d/chats?page=1");
    expect(result?.status).toBe(200);
    expect(result?.duration).toBe("280.2ms");
    expect(result?.remoteIp).toBe("::1");
    expect(result?.headers).toEqual([
      { name: "Accept", value: "*/*" },
      { name: "Connection", value: "close" },
      { name: "Host", value: "localhost:7119" },
    ]);
  });

  it("parses inbound HTTP access log with doubled quotes around query parameter", () => {
    const raw = `HTTP Inbound "GET" "/api/v1/me/businesses/01a1a0f4-dca1-7e41-9d00-f00c0c715e6d/chats""?page=1" returned 209 in 280.2ms
Remote IP: "::1"
Request Headers: [{"Accept": "*/*"}]`;

    const result = parseHttpLog(raw);
    expect(result).not.toBeNull();
    expect(result?.direction).toBe("Inbound");
    expect(result?.method).toBe("GET");
    expect(result?.path).toBe("/api/v1/me/businesses/01a1a0f4-dca1-7e41-9d00-f00c0c715e6d/chats?page=1");
    expect(result?.status).toBe(209);
    expect(result?.duration).toBe("280.2ms");
    expect(result?.remoteIp).toBe("::1");
  });

  it("parses single-line HTTP access log", () => {
    const raw = `HTTP "GET" "/api/v1/me/businesses/01a1a0f4-dca1-7e41-9d00-f00c0c715e6d/chats" responded 200 in 205.6758 ms`;
    const result = parseHttpLog(raw);
    expect(result).not.toBeNull();
    expect(result?.method).toBe("GET");
    expect(result?.path).toBe("/api/v1/me/businesses/01a1a0f4-dca1-7e41-9d00-f00c0c715e6d/chats");
    expect(result?.status).toBe(200);
    expect(result?.duration).toBe("205.7ms");
    expect(result?.remoteIp).toBeUndefined();
    expect(result?.headers).toEqual([]);
  });

  it("returns null for non-HTTP logs", () => {
    expect(parseHttpLog("Payment failed: card declined")).toBeNull();
    expect(parseHttpLog('{"message":"server error","code":500}')).toBeNull();
    expect(parseHttpLog("")).toBeNull();
  });
});

describe("normalizeSeverityText", () => {
  it("groups OTLP levels, preferring severity numbers over exporter text", () => {
    for (const [text, number, expected] of [
      ["ERROR4", 0, "ERROR"], ["WARNING", undefined, "WARN"], [" ", 0, "INFO"],
      ["Error", 9, "INFO"], ["custom", 20, "ERROR"], ["custom", 24, "FATAL"],
      ["custom", 25, "CUSTOM"], ["debug2", undefined, "DEBUG"],
    ] as const) expect(normalizeSeverityText(text, number)).toBe(expected);
  });

  it("normalizes INFORMATION and WARNING to short versions", () => {
    expect(normalizeSeverityText("INFORMATION")).toBe("INFO");
    expect(normalizeSeverityText("WARNING")).toBe("WARN");
    expect(normalizeSeverityText("error")).toBe("ERROR");
    expect(normalizeSeverityText(undefined)).toBe("INFO");
  });
});
