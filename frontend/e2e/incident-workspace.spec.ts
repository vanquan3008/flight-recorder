import { expect, test, type Page } from "@playwright/test";

const incidentID = "01a1a5be-6200-717d-9d4d-dbeba01f0d26";
const projectID = "01900000-0000-7000-8000-0000000000a1";
async function openIncident(page: Page) {
  const seen: string[] = [];
  const at = "2026-11-09T09:35:00.000Z";
  let resolved = false;
  const project = { id: projectID, name: "TracePilot", slug: "tracepilot", created_at: at };
  const record = { id: incidentID, title: "kalo-service observability baseline failed", severity: "ERROR", status: "OPEN", primary_service: "kalo-service", occurrence_count: 43, first_seen_at: "2026-11-09T08:48:00.000Z", last_seen_at: at, error_message: "Telemetry deviated from its baseline" };
  await page.addInitScript(() => localStorage.setItem("tracepilot-signed-in", "1"));
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    seen.push(url.pathname + url.search);
    let body: unknown = { data: [] };
    if (url.pathname === "/api/v1/auth/refresh") body = { user: { id: "operator", display_name: "Demo operator", email: "demo@example.test" }, memberships: [{ project, role: "owner" }], access_token: "synthetic-incident-token", expires_at: "2099-01-01T00:00:00Z" };
    else if (url.pathname === `/api/v1/projects/${projectID}`) body = { project, role: "owner", environments: [{ id: "prod", name: "production" }] };
    else if (url.pathname === `/api/v1/incidents/${incidentID}`) {
      if (route.request().method() === "PATCH") resolved = true;
      body = { ...record, status: resolved ? "RESOLVED" : "OPEN" };
    } else if (url.pathname.endsWith("/analysis")) body = {
      incident_id: incidentID, conclusion: "Highest-ranked hypothesis: DNS lookup duration increased compared with its baseline.",
      hypotheses: [{ rank: 1, entity_id: "service:kalo-service", statement: "DNS lookup duration increased for kalo-service. Inspect the supporting metric and correlated traces before treating this as the root cause.", confidence: 0.95, evidence_ids: ["evidence-dns"] }],
      evidence: [{ id: "evidence-dns", signal_type: "metric", entity_type: "service", entity_id: "service:kalo-service", metric_name: "dns.lookup.duration", series_id: "ffb84ff44354e5409a1b2a0331cc52d4", direction: "increase", observed_value: 0.001062, baseline_value: 0.0004316, change_ratio: 2.46, trace_ids: [], summary: "DNS lookup duration exceeded the previous observation window." }], causal_graph: [],
    };
    else if (url.pathname.endsWith("/occurrences")) {
      const start = Number(url.searchParams.get("cursor") ?? 0);
      body = { data: Array.from({ length: Math.min(25, 43 - start) }, (_, offset) => ({ id: `occurrence-${start + offset}`, occurred_at: new Date(Date.parse(at) - (start + offset) * 60000).toISOString(), service_name: "kalo-service", operation: `observability baseline ${start + offset + 1}`, error_message: "Telemetry deviated from its baseline", event_type: "metric" })), next_cursor: start === 0 ? "25" : undefined };
    } else if (url.pathname.endsWith("/status-changes")) body = { data: resolved ? [{ id: "change-1", previous_status: "OPEN", status: "RESOLVED", actor_user_id: "operator", changed_at: at }] : [] };
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(body) });
  });
  await page.goto(`/incidents/${incidentID}?project=${projectID}&environment=production`);
  await expect(page.getByRole("heading", { name: "RCA hypotheses" })).toBeVisible();
  return seen;
}

test("keeps the incident readable in both themes and loads records only when needed", async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 1600, height: 1000 });
  const seen = await openIncident(page);
  expect(seen.filter((path) => path.startsWith(`/api/v1/incidents/${incidentID}`))).toHaveLength(2);
  for (const theme of ["Light", "Dark"]) {
    await page.getByRole("button", { name: `${theme} theme` }).click();
    await expect(page.getByText("0.0004316", { exact: true })).toBeVisible();
    expect((await page.getByText("0.0004316", { exact: true }).boundingBox())!.y).toBeLessThan(900);
    await expect(page.getByText("Heuristic score 0.95", { exact: true })).toBeVisible();
    await page.screenshot({ path: testInfo.outputPath(`incident-${theme.toLowerCase()}.png`), animations: "disabled" });
  }
  await page.getByRole("button", { name: /View supporting evidence/ }).click();
  await expect(page.getByText("filtered by hypothesis", { exact: false })).toBeVisible();
  await page.getByRole("button", { name: "Clear selection" }).click();
  await page.getByRole("tab", { name: /Occurrences/ }).click();
  await expect(page.locator(".incident-record")).toHaveCount(25);
  await expect(page.getByText("observability baseline 1", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Next page", exact: true }).click();
  await expect(page.locator(".incident-record")).toHaveCount(18);
  await expect(page.getByText("observability baseline 26", { exact: true })).toBeVisible();
  await expect(page.getByText("observability baseline 1", { exact: true })).toHaveCount(0);
  await page.getByRole("tab", { name: "Status history", exact: true }).click();
  await expect(page.getByText("No status changes have been recorded.")).toBeVisible();
  await page.getByRole("button", { name: "Resolve incident", exact: true }).click();
  await expect(page.getByRole("button", { name: "Reopen incident", exact: true })).toBeVisible();
  await expect(page.locator(".incident-record")).toHaveCount(1);
});

test("supports mobile evidence inspection and keyboard navigation without horizontal overflow", async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openIncident(page);
  const tab = page.getByRole("tab", { name: "Investigation", exact: true });
  await tab.focus();
  await page.keyboard.press("ArrowRight");
  await expect(page.getByRole("tab", { name: /Occurrences/ })).toBeFocused();
  await expect(page.locator(".incident-record")).toHaveCount(25);
  await page.keyboard.press("Home");
  await expect(tab).toHaveAttribute("aria-selected", "true");
  await expect(page.getByRole("heading", { name: "Evidence", exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.body.scrollWidth)).toBeLessThanOrEqual(390);
  const summary = page.getByText("Signal details & trace links", { exact: true });
  await summary.click();
  await expect(page.getByText("Series: ffb84ff44354e5409a1b2a0331cc52d4", { exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.body.scrollWidth)).toBeLessThanOrEqual(390);
  await page.screenshot({ path: testInfo.outputPath("incident-mobile.png"), animations: "disabled" });
});
