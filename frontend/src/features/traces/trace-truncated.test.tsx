import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { App } from "../../app/app";

const traceID = "0123456789abcdef0123456789abcdef";
const span = {
  trace_id: traceID, span_id: "a", name: "GET /", service_name: "api", status_code: "OK",
  started_at: "2026-10-01T00:00:00.000Z", ended_at: "2026-10-01T00:00:00.010Z",
  attributes: {}, resource_attributes: {}, events: [], links: [],
};
const loaded = vi.hoisted(() => ({ truncated: true }));

vi.mock("./trace-pages", () => ({
  maxTraceSpans: 5000,
  loadTrace: () => Promise.resolve({ spans: [span], truncated: loaded.truncated }),
}));

afterEach(() => { cleanup(); sessionStorage.clear(); window.location.hash = ""; vi.unstubAllGlobals(); });

function open() {
  localStorage.setItem("tracepilot-signed-in", "1");
  window.location.hash = `#/traces/${traceID}`;
  vi.stubGlobal("fetch", vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify({ data: [] }), { status: 200 }))));
  render(<App />);
}

it("warns that a very large trace is incomplete instead of presenting it as whole", async () => {
  loaded.truncated = true;
  open();
  const notice = await screen.findByText(/more than 5,000 spans/);
  expect(notice.getAttribute("role")).toBe("status");
  expect(notice.textContent).toMatch(/incomplete/);
});

it("shows no warning for a complete trace", async () => {
  loaded.truncated = false;
  open();
  await screen.findByRole("heading", { name: "Request flow" });
  expect(screen.queryByText(/more than 5,000 spans/)).toBeNull();
});
