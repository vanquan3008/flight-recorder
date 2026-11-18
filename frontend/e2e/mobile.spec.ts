import { devices, expect, test, type Page } from "@playwright/test";

const projectID = "01900000-0000-7000-8000-0000000000a1";
const project = { id: projectID, name: "Checkout platform with a long project name", slug: "checkout", created_at: "2026-11-09T09:00:00Z" };
const traceID = "0123456789abcdef0123456789abcdef";
const at = Date.parse("2026-11-09T09:36:19.266Z");
const longPath = `/api/v1/me/businesses/01a1a0f4-dca1-7a41-9d00-f00c8c715e6d/chats?filter=${"very-long-request-path-".repeat(6)}`;
const logs = Array.from({ length: 30 }, (_, index) => ({
  event_id: `log-${index}`, occurred_at: new Date(at - index * 1000).toISOString(),
  service_name: index === 1 ? "kalo-service-with-a-very-long-deployment-name" : "checkout", severity_text: index % 5 === 0 ? "ERROR" : "INFO",
  body: `HTTP Inbound "GET" "${longPath}" returned ${index % 5 === 0 ? 500 : 200} in 8.4ms`, trace_id: traceID, span_id: `span-${index}`,
}));
const traces = Array.from({ length: 12 }, (_, index) => ({
  trace_id: `${traceID.slice(0, 30)}${String(index).padStart(2, "0")}`, started_at: new Date(at - index * 60_000).toISOString(),
  ended_at: new Date(at - index * 60_000 + 82).toISOString(), duration_nanos: 82_000_000, root_service: "kalo-service-with-a-very-long-deployment-name", span_count: 8, error_count: index % 3 ? 0 : 1,
}));

test.use({ ...devices["iPhone 13"], browserName: "chromium" });

async function openPhone(page: Page, path: string) {
  await page.addInitScript(() => localStorage.setItem("tracepilot-signed-in", "1"));
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    let body: unknown = { data: [] };
    if (url.pathname === "/api/v1/auth/refresh") body = { user: { id: "operator", display_name: "Demo operator", email: "demo@example.test" }, memberships: [{ project, role: "owner" }], access_token: "synthetic-mobile-token", expires_at: "2099-01-01T00:00:00Z" };
    else if (url.pathname === `/api/v1/projects/${projectID}`) body = { project, role: "owner", environments: [{ id: "prod", name: "production" }, { id: "stage", name: "staging" }] };
    else if (url.pathname === `/api/v1/projects/${projectID}/retention`) body = { retention_days: null, default_age_seconds: 604800, max_days: 106751 };
    else if (url.pathname === "/api/v1/logs") body = { data: logs };
    else if (url.pathname === "/api/v1/traces") body = { data: traces };
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(body) });
  });
  await page.goto(`${path}${path.includes("?") ? "&" : "?"}project=${projectID}&environment=production`);
  await expect(page.getByRole("navigation", { name: "Console sections" })).toBeVisible();
}

/** Every visible element stays inside the viewport unless a scrollable container deliberately holds it; clipping by overflow:hidden counts as a defect. */
const overflowing = (page: Page) => page.evaluate(() => {
  const width = document.documentElement.clientWidth;
  const clippedByScroller = (element: Element) => {
    for (let parent = element.parentElement; parent && parent !== document.body; parent = parent.parentElement) {
      if (/(auto|scroll)/.test(getComputedStyle(parent).overflowX) && parent.getBoundingClientRect().right <= width + 1) return true;
    }
    return false;
  };
  const offenders = [...document.querySelectorAll("body *")].filter((element) => {
    const box = element.getBoundingClientRect();
    return box.width > 0 && box.right > width + 1 && !clippedByScroller(element);
  }).map((element) => `${element.tagName.toLowerCase()}.${String(element.className).split(" ").slice(0, 2).join(".")}`);
  return { page: document.documentElement.scrollWidth - width, offenders: offenders.slice(0, 5) };
});

for (const path of ["/traces", "/logs", "/incidents", "/settings", `/traces/${traceID}`]) {
  test(`fits ${path} on a phone without sideways scrolling`, async ({ page }) => {
    await openPhone(page, path);
    await page.waitForLoadState("networkidle");
    expect(await overflowing(page)).toEqual({ page: 0, offenders: [] });
    const header = await page.locator(".topbar").boundingBox();
    expect(header?.height).toBeLessThanOrEqual(64);
  });
}

test("keeps every section and the account reachable from the bottom bar", async ({ page }) => {
  await openPhone(page, "/traces");
  const bar = page.getByRole("navigation", { name: "Console sections" });
  await expect(bar.getByRole("link", { name: "Traces" })).toHaveAttribute("aria-current", "page");
  await bar.getByRole("button", { name: "More", exact: true }).click();
  const sheet = page.getByRole("dialog", { name: "More" });
  await expect(sheet.getByRole("button", { name: "Sign out" })).toBeVisible();
  await sheet.getByRole("link", { name: "Settings" }).click();
  await expect(page).toHaveURL(/\/settings\?/);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(bar.getByRole("button", { name: "More", exact: true })).toHaveAttribute("aria-current", "page");
});

test("filters logs from the sheet and keeps the scope in the header chip", async ({ page }) => {
  await openPhone(page, "/logs");
  await expect(page.getByRole("button", { name: /^Scope: Checkout platform/ })).toBeVisible();
  await page.getByRole("button", { name: /^Filters/ }).click();
  const sheet = page.getByRole("dialog", { name: "Filter logs" });
  await sheet.getByLabel("Service").fill("checkout");
  await sheet.getByRole("button", { name: "Apply" }).click();
  await expect(page).toHaveURL(/service=checkout/);
  await expect(page.getByRole("button", { name: /^Filters 1/ })).toBeVisible();
  await expect(page.locator(".log-table-row").first()).toBeVisible();
});
