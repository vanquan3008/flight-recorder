import { expect, test, type Page } from "@playwright/test";

const projectID = "01900000-0000-7000-8000-0000000000a1";
const traceID = "0123456789abcdef0123456789abcdef";
const linkedTraceID = "fedcba9876543210fedcba9876543210";
const project = { id: projectID, slug: "production", name: "TracePilot Demo", created_at: "2026-11-09T09:00:00Z" };

// Attribute values arrive as strings: the API stores them as ClickHouse Map(String, String).
const spans = [
  { trace_id: traceID, span_id: "1111111111111111", name: "GET /cache", kind: 2, service_name: "api", service_version: "3.1.0", status_code: "OK",
    started_at: "2026-11-09T09:36:19.000000000Z", ended_at: "2026-11-09T09:36:19.002000000Z", duration_nanos: 2_000_000,
    attributes: { "http.request.method": "GET", "http.route": "/cache", "http.response.status_code": "200" }, resource_attributes: { "service.name": "api" }, events: [], links: [] },
  { trace_id: traceID, span_id: "2222222222222222", parent_span_id: "1111111111111111", name: "redis GET", kind: 3, service_name: "api", status_code: "OK",
    started_at: "2026-11-09T09:36:19.000500000Z", ended_at: "2026-11-09T09:36:19.000750000Z", duration_nanos: 250_000,
    scope_name: "io.opentelemetry.redis", scope_version: "1.4.0", dropped_attributes_count: 2,
    attributes: { "db.system": "redis" }, resource_attributes: { "service.name": "api" }, events: [],
    links: [{ TraceID: linkedTraceID, SpanID: "3333333333333333", TraceState: "", Flags: 1, Attributes: { "messaging.operation": "process" }, DroppedAttributes: 0 }] },
];
const log = {
  event_id: "log-1", occurred_at: "2026-11-09T09:36:19.001Z", service_name: "api", severity_text: "ERROR", severity_number: 17,
  body: "cache refresh failed", trace_id: traceID, span_id: "2222222222222222", resource_attributes: { "service.name": "api" },
  attributes: { "cache.key": "orders:42", "http.response.status_code": "503" }, dropped_attributes_count: 1,
};

async function mockAPI(page: Page) {
  await page.addInitScript(() => localStorage.setItem("tracepilot-signed-in", "1"));
  await page.route("**/api/v1/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    let body: unknown = { data: [] };
    if (path === "/api/v1/auth/refresh") body = { user: { id: "operator", display_name: "Demo operator", email: "demo@example.test" }, memberships: [{ project, role: "owner" }], access_token: "synthetic-token", expires_at: "2099-01-01T00:00:00Z" };
    else if (path === `/api/v1/projects/${projectID}`) body = { project, role: "owner", environments: [{ id: "prod", name: "production" }] };
    else if (path === `/api/v1/traces/${traceID}`) body = { trace_id: traceID, spans };
    else if (path === `/api/v1/traces/${traceID}/logs` || path === "/api/v1/logs") body = { data: [log] };
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(body) });
  });
}

test("measures sub-millisecond spans and follows span links", async ({ page }) => {
  await page.setViewportSize({ width: 1600, height: 1000 });
  await mockAPI(page);
  await page.goto(`/traces/${traceID}?project=${projectID}&environment=production&range=1h`);
  await expect(page.getByRole("heading", { name: "Span waterfall" })).toBeVisible();
  const child = page.locator('[data-span-id="2222222222222222"]');
  await expect(child).toContainText("250 µs");
  await child.locator(".wf-main").click();
  const inspector = page.getByRole("complementary", { name: "Selected span inspector" });
  const facts = inspector.getByLabel("Span facts");
  await expect(facts).toContainText("Client");
  await expect(facts).toContainText("io.opentelemetry.redis 1.4.0");
  await expect(inspector.getByRole("note")).toContainText("2 attributes");
  await inspector.getByRole("tab", { name: /Links/ }).click();
  await expect(inspector.getByText("Span 3333333333333333")).toBeVisible();
  const link = inspector.getByRole("link", { name: /Open linked trace/ });
  await expect(link).toHaveAttribute("href", new RegExp(`/traces/${linkedTraceID}\\?.*project=${projectID}`));
  await expect(link).toHaveAttribute("href", /range=1h/);
});

test("shows a log's own attributes next to its resource attributes", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await mockAPI(page);
  await page.goto(`/logs?project=${projectID}&environment=production`);
  await expect(page.locator(".log-table-row")).toHaveCount(1);
  await page.getByRole("button", { name: "Expand log details" }).click();
  const details = page.locator(".log-detail-row");
  await expect(details.getByText("Log attributes (2)")).toBeVisible();
  await expect(details.getByText("orders:42")).toBeVisible();
  await expect(details.getByRole("note")).toContainText("1 attribute");
});
