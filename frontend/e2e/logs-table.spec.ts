import { expect, test, type Page } from "@playwright/test";

const projectID = "01900000-0000-7000-8000-0000000000a1";
const traceID = "0123456789abcdef0123456789abcdef";
async function openLogs(page: Page) {
  const at = Date.parse("2026-11-09T09:36:19.266Z");
  const logs = Array.from({ length: 105 }, (_, index) => {
    const method = ["GET", "POST", "DELETE"][index % 3];
    const status = index % 11 === 0 ? 500 : 200;
    const path = `/api/v1/me/businesses/01a1a0f4-dca1-7a41-9d00-f00c8c715e6d/chats/${index}?page=1&filter=${"very-long-request-path-".repeat(index === 0 ? 14 : 1)}`;
    return { event_id: `log-${index}`, occurred_at: new Date(at - index * 5).toISOString(), service_name: index === 1 ? "kalo-service-with-a-very-long-deployment-name" : "kalo-service", severity_text: status === 500 ? "ERROR" : index % 7 === 0 ? "WARN" : "INFORMATION",
      body: index === 3 ? `Audit message with a long payload\n${"property=value; ".repeat(150)}` : `HTTP Inbound "${method}" "${path}" returned ${status} in ${index % 2 ? "8.4" : "288.2"}ms\nRemote IP: "::1"\nRequest Headers: [{"Accept":"application/json"}]`,
      trace_id: index % 3 ? traceID : undefined, span_id: `span-${index}`, resource_attributes: { "service.version": "1.0", "deployment.environment": "production" } };
  });
  const project = { id: projectID, name: "TracePilot", slug: "tracepilot", created_at: new Date(at).toISOString() };
  await page.addInitScript(() => {
    localStorage.setItem("tracepilot-signed-in", "1");
    let copied = "";
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText: async (value: string) => { copied = value; }, readText: async () => copied } });
  });
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    let body: unknown = { data: [] };
    if (url.pathname === "/api/v1/auth/refresh") body = { user: { id: "operator", display_name: "Demo operator", email: "demo@example.test" }, memberships: [{ project, role: "owner" }], access_token: "synthetic-logs-token", expires_at: "2099-01-01T00:00:00Z" };
    else if (url.pathname === `/api/v1/projects/${projectID}`) body = { project, role: "owner", environments: [{ id: "prod", name: "production" }] };
    else if (url.pathname === "/api/v1/logs") body = url.searchParams.has("cursor") ? { data: logs.slice(100) } : { data: logs.slice(0, 100), next_cursor: "next-100" };
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(body) });
  });
  await page.goto(`/logs?project=${projectID}&environment=production`);
  await expect(page.locator(".log-table-row")).toHaveCount(100);
  return logs;
}

for (const theme of ["Light", "Dark"]) {
  test(`aligns 100 log rows, HTTP fields and copy controls in ${theme} theme`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width: 1600, height: 1000 });
    const logs = await openLogs(page);
    await page.getByRole("button", { name: `${theme} theme` }).click();
    const aligned = await page.locator(".log-table").evaluate((table) => {
      const headers = [...table.querySelectorAll("thead th")].map((cell) => cell.getBoundingClientRect());
      return [...table.querySelectorAll(".log-table-row")].slice(0, 20).every((row) => {
        const cells = [...row.querySelectorAll("td")].map((cell) => cell.getBoundingClientRect());
        return cells.length === headers.length && cells.every((cell, index) => Math.abs(cell.left - headers[index].left) < 1.1 && Math.abs(cell.width - headers[index].width) < 1.1) && Math.abs(row.getBoundingClientRect().height - 48) < 1.1;
      });
    });
    expect(aligned).toBe(true);
    const pathStarts = await page.locator(".log-http-preview .log-path").evaluateAll((items) => items.slice(0, 10).map((item) => item.getBoundingClientRect().left));
    expect(Math.max(...pathStarts) - Math.min(...pathStarts)).toBeLessThan(1);
    const actions = page.locator(".log-table-row").first().locator(".log-col-actions");
    const button = actions.getByRole("button", { name: "Copy log body" });
    const cellBox = (await actions.boundingBox())!, buttonBox = (await button.boundingBox())!, viewport = (await page.getByRole("region", { name: "Log records, scrollable" }).boundingBox())!;
    expect(buttonBox.x).toBeGreaterThanOrEqual(cellBox.x);
    expect(buttonBox.x + buttonBox.width).toBeLessThanOrEqual(Math.min(cellBox.x + cellBox.width, viewport.x + viewport.width));
    await button.click();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(logs[0].body);
    await expect(page.locator(".log-detail-row")).toHaveCount(0);
    await page.screenshot({ path: testInfo.outputPath(`logs-${theme.toLowerCase()}.png`), animations: "disabled" });
    const rawRow = page.locator('[data-log-id="log-3"]');
    await rawRow.getByRole("button", { name: "Expand log details" }).click();
    await expect(page.locator(".log-detail-row pre")).toHaveText(logs[3].body);
    await page.screenshot({ path: testInfo.outputPath(`logs-expanded-${theme.toLowerCase()}.png`), animations: "disabled" });
    await page.getByRole("region", { name: "Log records, scrollable" }).evaluate((element) => { element.scrollTop = element.scrollHeight; });
    await expect(page.locator(".log-table-row")).toHaveCount(105);
  });
}

test("keeps every column reachable on mobile without clipping page content", async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openLogs(page);
  const viewport = page.getByRole("region", { name: "Log records, scrollable" });
  expect(await page.evaluate(() => document.body.scrollWidth)).toBeLessThanOrEqual(390);
  expect(await viewport.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(true);
  await viewport.evaluate((element) => { element.scrollLeft = element.scrollWidth; });
  await expect(page.locator(".log-table-row").first().getByRole("button", { name: "Copy log body" })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath("logs-mobile.png"), animations: "disabled" });
});