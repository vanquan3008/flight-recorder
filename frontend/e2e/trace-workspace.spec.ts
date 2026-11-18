import { expect, test, type Page } from "@playwright/test";

const traceID = "0123456789abcdef0123456789abcdef";
const projectID = "01900000-0000-7000-8000-0000000000a1";
const queryText = "SELECT t.id, t.auto_migration, t.connection_string, t.created_at,\n       t.deleted_at, t.host_domain, t.is_active, t.merchant_id\nFROM tenant AS t\nWHERE t.is_active";

async function openTrace(page: Page, kind: "single" | "distributed" | "database" = "single") {
  const base = Date.parse("2026-11-09T09:36:19.000Z");
  const span = (id: string, name: string, service: string, from: number, duration: number, parent?: string) => ({
    trace_id: traceID, span_id: id, parent_span_id: parent, name, service_name: service, status_code: "OK",
    started_at: new Date(base + from).toISOString(), ended_at: new Date(base + from + duration).toISOString(),
    attributes: { "http.request.method": "GET", "http.response.status_code": 200, "http.route": "/api/v1/chats/{chatId:guid}/members", "server.address": "api.internal", "url.scheme": "https", "network.protocol.version": "1.1" },
    resource_attributes: { "service.name": service, "service.version": "2.4.0", "deployment.environment": "production", "telemetry.sdk.language": "dotnet" }, events: [], links: [],
  });
  const spans = kind === "distributed" ? [
    span("root", "POST /checkout", "gateway", 0, 650),
    span("orders", "POST /orders", "order-service", 20, 580, "root"),
    span("db", "SELECT orders", "postgres", 50, 100, "orders"),
    { ...span("failed", "POST /payments", "payment-service", 180, 400, "orders"), status_code: "ERROR", attributes: { "http.response.status_code": 502, "exception.type": "TimeoutException", "exception.message": "Payment provider did not answer within the request deadline." } },
  ] : kind === "database" ? [
    span("root", "POST /api/webhooks/facebook", "checkout-api", 0, 44),
    ...Array.from({ length: 56 }, (_, index) => ({
      ...span(`db-${index}`, "postgresql", "checkout-api", index % 40, 1, "root"),
      attributes: { "db.namespace": "tenant", "db.query.text": queryText, ...Object.fromEntries(Array.from({ length: 20 }, (_, i) => [`db.field.${i}`, `value-${i}`])) },
      events: [{ name: "query completed", attributes: { rows: 1 } }],
    })),
  ] : [span("root", "GET /api/v1/chats/{chatId:guid}/members", "kalo-service", 0, 65)];
  const project = { id: projectID, slug: "production", name: "TracePilot Demo", created_at: new Date(base).toISOString() };
  const logs = [
    { event_id: "log-1", occurred_at: new Date(base + 65).toISOString(), service_name: spans[0].service_name, severity_text: "INFORMATION", body: `HTTP inbound GET /api/v1/chats/{chatId}/members returned 200 in 65ms.\n${"Request header: synthetic-example; ".repeat(70)}`, trace_id: traceID, span_id: "root", resource_attributes: {} },
    { event_id: "log-2", occurred_at: new Date(base + 62).toISOString(), service_name: spans[0].service_name, severity_text: "INFO", body: "HTTP GET /api/v1/chats/{chatId}/members responded 200 in 48.01ms", trace_id: traceID, span_id: "root", resource_attributes: {} },
  ];
  await page.addInitScript(() => {
    localStorage.setItem("tracepilot-signed-in", "1");
    let copied = "";
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText: async (text: string) => { copied = text; }, readText: async () => copied } });
  });
  await page.route("**/api/v1/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    let data: unknown = { data: [] };
    if (path === "/api/v1/auth/refresh") data = { user: { id: "audit", display_name: "Demo operator", email: "demo@example.test" }, memberships: [{ project, role: "owner" }], access_token: "synthetic-ui-token", expires_at: "2099-01-01T00:00:00Z" };
    else if (path === `/api/v1/projects/${projectID}`) data = { project, role: "owner", environments: [{ id: "prod", name: "production" }] };
    else if (path === `/api/v1/traces/${traceID}`) data = { trace_id: traceID, spans };
    else if (path === `/api/v1/traces/${traceID}/logs`) data = { data: logs };
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(data) });
  });
  await page.goto(`/traces/${traceID}?project=${projectID}&environment=production`);
  await expect(page.getByRole("heading", { name: "Span waterfall" })).toBeVisible();
  await expect(page.locator("details")).toHaveCount(2);
}

test("keeps a single-span request compact and shows its details alongside the timeline", async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 1600, height: 1000 });
  await openTrace(page);
  await page.getByRole("button", { name: "Light theme" }).click();
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("GET /api/v1/chats/{chatId:guid}/members");
  await expect(page.getByText("Single-service trace")).toBeVisible();
  await expect(page.getByRole("button", { name: "Service map" })).toHaveCount(0);
  const logsBox = await page.getByRole("heading", { name: "Correlated logs" }).boundingBox();
  expect(logsBox!.y).toBeLessThan(720);
  await page.screenshot({ path: testInfo.outputPath("trace-single-light.png"), animations: "disabled" });
  await page.locator(".wf-main").click();
  const inspector = page.getByRole("complementary", { name: "Selected span inspector" });
  await expect(inspector).toBeVisible();
  const viewport = await page.locator(".wf-viewport").boundingBox(), detail = await inspector.boundingBox();
  expect(detail!.x).toBeGreaterThanOrEqual(viewport!.x + viewport!.width - 1);
  await page.getByRole("tab", { name: /Resource/ }).click();
  await expect(page.getByRole("tab", { name: /Resource/ })).toHaveCSS("border-bottom-width", "2px");
  await expect(page.getByRole("tab", { name: /Resource/ })).toHaveCSS("border-bottom-style", "solid");
  await expect(page.getByRole("tab", { name: /Resource/ })).toHaveCSS("border-top-width", "0px");
  await expect(page.getByText("service.version", { exact: true })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath("trace-inspector-light.png"), animations: "disabled" });
  await page.getByRole("button", { name: "Dark theme" }).click();
  await page.screenshot({ path: testInfo.outputPath("trace-inspector-dark.png"), animations: "disabled" });
});

test("supports error triage, span search and keyboard inspector tabs", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await openTrace(page, "distributed");
  await page.getByRole("button", { name: "Collapse all" }).click();
  await expect(page.locator(".wf-row")).toHaveCount(1);
  await page.getByRole("button", { name: /^Errors/ }).click();
  await expect(page.locator(".wf-row")).toHaveCount(1);
  await expect(page.locator(".wf-name")).toHaveText("POST /payments");
  await page.locator(".wf-main").click();
  await expect(page.getByText("TimeoutException", { exact: true }).first()).toBeVisible();
  await page.getByRole("tab", { name: /Attributes/ }).focus();
  await page.keyboard.press("ArrowRight");
  await expect(page.getByRole("tab", { name: /Resource/ })).toBeFocused();
  await expect(page.getByRole("tab", { name: /Resource/ })).toHaveAttribute("aria-selected", "true");
  await page.getByRole("button", { name: "Close span details" }).click();
  await page.getByRole("button", { name: "All spans", exact: true }).click();
  await page.getByRole("searchbox", { name: "Search spans" }).fill("SELECT orders");
  await expect(page.locator(".wf-row")).toHaveCount(1);
  await expect(page.locator(".wf-name")).toHaveText("SELECT orders");
});

test("gives long SQL the inspector width and keeps its close control visible while scrolling", async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 1807, height: 822 });
  await openTrace(page, "database");
  await page.getByRole("button", { name: "Dark theme" }).click();
  await page.locator(".waterfall").evaluate((waterfall) => waterfall.scrollIntoView({ block: "start" }));
  const opener = page.locator('[data-span-id="db-0"] .wf-main');
  await opener.click();
  const inspector = page.getByRole("complementary", { name: "Selected span inspector" });
  const close = inspector.getByRole("button", { name: "Close span details" });
  const sql = inspector.getByText(queryText, { exact: true });
  await expect(sql).toHaveCSS("white-space", "pre-wrap");
  await expect(sql).toHaveCSS("overflow-wrap", "anywhere");
  await inspector.getByRole("button", { name: "Copy db.query.text" }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(queryText);
  const axis = (await page.locator(".wf-axis").boundingBox())!;
  const rail = (await page.locator('[data-span-id="db-0"] .wf-rail').boundingBox())!;
  expect(Math.abs(axis.x - rail.x)).toBeLessThan(1);
  expect(Math.abs(axis.width - rail.width)).toBeLessThan(1);
  const panel = (await inspector.boundingBox())!;
  const sqlRow = (await sql.locator("..").boundingBox())!;
  expect(sqlRow.width).toBeGreaterThan(panel.width * 0.85);
  const logs = (await page.getByRole("region", { name: "Correlated logs" }).boundingBox())!;
  expect(logs.y).toBeGreaterThanOrEqual(panel.y + panel.height - 1);
  expect(logs.x + logs.width).toBeGreaterThanOrEqual(panel.x + panel.width - 1);
  await expect(close).toBeInViewport();
  await page.screenshot({ path: testInfo.outputPath("trace-sql-desktop.png"), animations: "disabled" });
  const closeTop = (await close.boundingBox())!.y;
  await inspector.evaluate((element) => { element.scrollTop = element.scrollHeight; });
  expect(await inspector.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  expect((await close.boundingBox())!.y).toBeCloseTo(closeTop, 0);
  await close.click();
  await expect(opener).toBeFocused();
});

test("keeps long logs and the inspector readable on mobile", async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openTrace(page);
  const opener = page.locator(".wf-main");
  await opener.click();
  const inspector = page.getByRole("dialog", { name: "Span details" });
  await expect(inspector).toBeVisible();
  expect(await page.evaluate(() => document.body.scrollWidth)).toBeLessThanOrEqual(390);
  await page.keyboard.press("Escape");
  await expect(inspector).toHaveCount(0);
  await expect(opener).toBeFocused();
  await page.getByRole("textbox", { name: "Filter loaded logs" }).fill("48.01");
  await expect(page.locator("details")).toHaveCount(1);
  await page.getByRole("textbox", { name: "Filter loaded logs" }).fill("");
  await page.locator("details summary").first().click();
  await expect(page.locator("details[open] pre")).toContainText("synthetic-example");
  expect(await page.evaluate(() => document.body.scrollWidth)).toBeLessThanOrEqual(390);
  await page.screenshot({ path: testInfo.outputPath("trace-mobile.png"), animations: "disabled" });
});

test("keeps SQL, property tabs and the timeline reachable on a narrow phone", async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 320, height: 660 });
  await openTrace(page, "database");
  const opener = page.locator('[data-span-id="db-0"] .wf-main');
  await opener.click();
  const inspector = page.getByRole("dialog", { name: "Span details" });
  await expect(inspector.getByText(queryText, { exact: true })).toBeVisible();
  expect(await inspector.evaluate((el) => el.scrollWidth)).toBeLessThanOrEqual(320);
  await page.screenshot({ path: testInfo.outputPath("trace-sql-mobile.png"), animations: "disabled" });
  await inspector.getByRole("tab", { name: /Events/ }).click();
  await expect(inspector.getByText("query completed")).toBeVisible();
  await inspector.getByRole("button", { name: "Close", exact: true }).click();
  await expect(opener).toBeFocused();
  await expect(page.locator(".wf-viewport")).toBeInViewport();

  await page.setViewportSize({ width: 1024, height: 780 });
  await opener.click();
  const sheet = (await inspector.boundingBox())!;
  expect(sheet.width).toBeLessThan(1024);
  expect(sheet.x * 2 + sheet.width).toBeCloseTo(1024, 0);
});
