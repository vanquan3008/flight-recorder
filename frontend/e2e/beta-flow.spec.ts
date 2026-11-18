import { expect, test, type Page } from "@playwright/test";

const projectID = "01900000-0000-7000-8000-0000000000a1";
const traceID = "0123456789abcdef0123456789abcdef";
type Seen = { path: string; headers: Record<string, string> };

async function mockConsole(page: Page) {
  const seen: Seen[] = [];
  const now = new Date().toISOString();
  const project = { id: projectID, slug: "alpha", name: "Alpha", created_at: now };
  const session = {
    user: { id: "audit", email: "audit@example.test", display_name: "Audit", platform_admin: false, must_change_password: false, created_at: now },
    memberships: [{ project, role: "owner" }], access_token: "browser-audit-jwt", expires_at: new Date(Date.now() + 3600000).toISOString(),
  };
  const log = { event_id: "shared-event", occurred_at: now, service_name: "checkout", severity_text: "ERROR", body: "matching error", trace_id: traceID, span_id: "span", resource_attributes: {} };
  const base = Date.now();
  const spans = Array.from({ length: 5000 }, (_, i) => ({
    trace_id: traceID, span_id: i.toString(16).padStart(16, "0"), parent_span_id: i ? "0000000000000000" : undefined,
    name: `GET /orders/${i}`, service_name: "checkout", status_code: "OK",
    started_at: new Date(base + i).toISOString(), ended_at: new Date(base + (i === 0 ? 5100 : i + 10)).toISOString(),
    attributes: i === 1 ? Object.fromEntries(Array.from({ length: 40 }, (_, n) => [`field${n}`, `value${n}`])) : {},
    resource_attributes: {}, events: [], links: [],
  }));
  await page.addInitScript(() => localStorage.setItem("tracepilot-signed-in", "1"));
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request(), url = new URL(request.url());
    seen.push({ path: url.pathname + url.search, headers: request.headers() });
    let body: unknown = { data: [] };
    if (url.pathname === "/api/v1/auth/refresh") body = session;
    else if (url.pathname === `/api/v1/projects/${projectID}`) body = { project, role: "owner", environments: [{ id: "env-dev", name: "development" }, { id: "env-prod", name: "production" }] };
    else if (url.pathname === "/api/v1/traces") body = { data: [{ trace_id: traceID, started_at: now, root_service: "checkout", duration_nanos: 5100000000, span_count: 5000, error_count: 0 }] };
    else if (url.pathname === `/api/v1/traces/${traceID}`) {
      const offset = Number(url.searchParams.get("cursor") || "0");
      body = { trace_id: traceID, spans: spans.slice(offset, offset + 500), next_cursor: offset + 500 < spans.length ? String(offset + 500) : undefined };
    } else if (url.pathname === "/api/v1/logs") body = { data: [log] };
    else if (url.pathname === "/api/v1/topology") body = {
      nodes: [{ id: "checkout", name: "checkout", type: "SERVICE" }, { id: "payments", name: "payments", type: "SERVICE" }],
      edges: [{ source_node_id: "checkout", target_node_id: "payments", kind: "HTTP", trace_ids: [], call_count: 120, error_count: 2, average_latency_nanos: 1e6, p95_latency_nanos: 2e6 }],
    };
    else if (url.pathname === "/api/v1/telemetry/quality") body = { traces: 1, spans: 5000, logs: 1, metric_points: 10 };
    else if (url.pathname === "/api/v1/performance") {
      const stats = { requests: 1000, errors: 10, requests_per_second: 1, error_rate: 0.01, average_latency_nanos: 1e6, p95_latency_nanos: 2e6, p99_latency_nanos: 3e6 };
      const from = new Date(base - 900000).toISOString();
      body = { from, to: now, summary: stats, series: [{ ...stats, from, to: now }],
        services: [{ ...stats, service: "checkout" }], endpoints: [{ ...stats, service: "checkout", endpoint: "GET /orders" }], limit: 50 };
    }
    else if (url.pathname === "/api/v1/live/stream") {
      const events = [log, { ...log, event_id: "live-event", body: "matching live event" }, { ...log, event_id: "filtered-event", body: "unrelated live event", span_id: "other-span" }];
      await route.fulfill({ contentType: "text/event-stream", body: events.map((payload) => `data: ${JSON.stringify({ type: "log", payload })}\n\n`).join("") });
      return;
    }
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(body) });
  });
  return seen;
}

test("live tail authenticates in its scope, filters events and deduplicates persisted logs", async ({ page }) => {
  const seen = await mockConsole(page);
  await page.goto(`/logs?project=${projectID}&environment=production&service=checkout&severity=ERROR&spanId=span&search=matching`);
  await page.getByRole("button", { name: "Live tail", exact: true }).click();
  await expect(page.getByText("matching live event", { exact: true })).toBeVisible();
  await expect(page.getByText("matching error", { exact: true })).toHaveCount(1);
  await expect(page.getByText("unrelated live event", { exact: true })).toHaveCount(0);
  const stream = seen.find((call) => call.path === "/api/v1/live/stream")!;
  expect(stream.headers.authorization).toBe("Bearer browser-audit-jwt");
  expect(stream.headers["x-project-id"]).toBe(projectID);
  expect(stream.headers["x-environment"]).toBe("production");
});

test("keeps a 24-hour window and environment through topology, logs and traces", async ({ page }) => {
  const seen = await mockConsole(page);
  await page.goto(`/topology?project=${projectID}&range=24h&to=2026-10-02T00:00:00Z`);
  await page.getByRole("combobox", { name: "Environment", exact: true }).click();
  await page.getByRole("option", { name: "production", exact: true }).click();
  await page.getByRole("link", { name: /Open in Log explorer/ }).click();
  await expect(page.getByRole("heading", { name: "Log explorer" })).toBeVisible();
  await expect.poll(() => seen.filter((call) => call.path.startsWith("/api/v1/logs?")).at(-1)?.headers["x-environment"]).toBe("production");
  const logs = new URL(seen.filter((call) => call.path.startsWith("/api/v1/logs?")).at(-1)!.path, "http://localhost");
  expect(logs.searchParams.get("from")).toBe("2026-10-01T00:00:00.000Z");
  expect(logs.searchParams.get("to")).toBe("2026-10-02T00:00:00.000Z");
  await page.getByRole("link", { name: "Trace explorer", exact: true }).click();
  await expect(page.getByRole("radio", { name: "Last 24 hours" })).toBeChecked();
  expect(new URL(page.url()).searchParams.get("environment")).toBe("production");
});

test("renders 5000 spans with bounded rows and an inspector that cannot overlap them", async ({ page }) => {
  await mockConsole(page);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto(`/traces/${traceID}?project=${projectID}`);
  await expect(page.locator(".wf-viewport")).toBeVisible();
  expect(await page.locator(".wf-row").count()).toBeLessThan(100);
  await page.locator(".wf-row .wf-main").nth(1).click();
  await expect(page.locator(".wf-inspector")).toBeVisible();
  expect(await page.locator(".wf-row [data-span-detail]").count()).toBe(0);
  const overlap = await page.locator(".wf-row").evaluateAll((rows) => rows.slice(1).some((row, index) => rows[index].getBoundingClientRect().bottom > row.getBoundingClientRect().top + 0.5));
  expect(overlap).toBe(false);
  await page.getByRole("button", { name: "Close span details" }).click();
  await page.locator(".wf-viewport").evaluate((element) => { element.scrollTop = element.scrollHeight; });
  await expect(page.locator(".wf-main").filter({ hasText: "GET /orders/4999" })).toBeVisible();
  expect(await page.locator(".wf-row").count()).toBeLessThan(100);
  await page.setViewportSize({ width: 390, height: 844 });
  const mobileOverlap = await page.locator(".wf-row").evaluateAll((rows) => rows.slice(1).some((row, index) => rows[index].getBoundingClientRect().bottom > row.getBoundingClientRect().top + 0.5));
  expect(mobileOverlap).toBe(false);
});

/** Phones keep the theme under More; wider screens show it in the header. */
async function chooseTheme(page: Page, theme: string) {
  const more = page.getByRole("navigation", { name: "Console sections" }).getByRole("button", { name: "More", exact: true });
  await expect(more.or(page.getByRole("group", { name: "Color theme" }))).toBeVisible(); // the session has been restored
  const onPhone = await more.isVisible();
  if (onPhone) await more.click();
  await page.getByRole("button", { name: `${theme} theme` }).click();
  if (onPhone) await page.keyboard.press("Escape");
}

test("keeps the scope chip and the bottom bar usable on mobile in both themes", async ({ page }, testInfo) => {
  await mockConsole(page);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`/overview?project=${projectID}&environment=production`);
  await expect(page.getByRole("heading", { name: "Errors observed in this window" })).toBeVisible();
  const header = await page.locator(".topbar").boundingBox();
  expect(header!.height).toBeLessThanOrEqual(64);
  expect(await page.evaluate(() => document.body.scrollWidth)).toBeLessThanOrEqual(390);
  await expect(page.getByRole("button", { name: /^Scope: Alpha, production\./ })).toBeVisible();
  await expect(page.getByRole("button", { name: "Toggle navigation" })).toHaveCount(0);
  for (const theme of ["Dark", "Light"]) {
    await chooseTheme(page, theme);
    await expect(page.locator("html")).toHaveAttribute("data-theme", theme.toLowerCase());
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await page.screenshot({ path: testInfo.outputPath(`mobile-${theme.toLowerCase()}.png`), animations: "disabled" });
  }
  const bar = page.getByRole("navigation", { name: "Console sections" });
  await expect(bar).toBeInViewport();
  await bar.getByRole("link", { name: "Logs" }).click();
  await expect(page.getByRole("heading", { name: "Log explorer" })).toBeVisible();
  expect(new URL(page.url()).searchParams.get("environment")).toBe("production");
});

test("shows themed scope dropdowns above the page and keeps keyboard selection scoped", async ({ page }, testInfo) => {
  const seen = await mockConsole(page);
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto(`/logs?project=${projectID}&range=24h`);
  const trigger = page.getByRole("combobox", { name: "Environment", exact: true });
  for (const theme of ["Light", "Dark"]) {
    await page.getByRole("button", { name: `${theme} theme` }).click();
    await trigger.click();
    await expect(page.getByRole("option", { name: "Default environment", exact: true })).toHaveAttribute("aria-selected", "true");
    const menu = await page.getByRole("listbox", { name: "Environment options" }).boundingBox();
    const topbar = await page.locator(".topbar").boundingBox();
    expect(menu!.y).toBeGreaterThan(topbar!.y);
    expect(menu!.width).toBeGreaterThanOrEqual(260);
    await page.screenshot({ path: testInfo.outputPath(`scope-dropdown-${theme.toLowerCase()}.png`), animations: "disabled" });
    await page.keyboard.press("Escape");
    await expect(trigger).toBeFocused();
    await expect(page.getByRole("listbox")).toHaveCount(0);
  }
  await trigger.focus();
  await page.keyboard.press("p");
  await page.keyboard.press("Enter");
  await expect.poll(() => seen.filter((call) => call.path.startsWith("/api/v1/logs?")).at(-1)?.headers["x-environment"]).toBe("production");
  expect(new URL(page.url()).searchParams.get("range")).toBe("24h");
  await trigger.click();
  await page.getByRole("option", { name: "Default environment", exact: true }).click();
  await expect.poll(() => new URL(page.url()).searchParams.get("environment")).toBeNull();
  await page.getByRole("combobox", { name: "Project", exact: true }).click();
  await expect(page.getByRole("option", { name: "Alpha", exact: true })).toHaveAttribute("aria-selected", "true");
  await page.getByRole("heading", { name: "Log explorer" }).click();
  await expect(page.getByRole("listbox")).toHaveCount(0);
});

test("keeps the scope sheet inside narrow mobile viewports", async ({ page }, testInfo) => {
  await mockConsole(page);
  for (const width of [390, 320]) {
    await page.setViewportSize({ width, height: 844 });
    await page.goto(`/logs?project=${projectID}`);
    await page.getByRole("button", { name: /^Scope: Alpha/ }).click();
    const sheet = page.getByRole("dialog", { name: "Project and environment" });
    const box = await sheet.boundingBox();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(width);
    expect(await page.evaluate(() => document.body.scrollWidth)).toBeLessThanOrEqual(width);
    await page.screenshot({ path: testInfo.outputPath(`scope-sheet-mobile-${width}.png`), animations: "disabled" });
    await sheet.getByRole("button", { name: "development", exact: true }).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect.poll(() => new URL(page.url()).searchParams.get("environment")).toBe("development");
  }
});

test("keeps log filters, environment actions and topology controls inside their panels", async ({ page }) => {
  await mockConsole(page);
  for (const width of [768, 901]) {
    await page.setViewportSize({ width, height: 780 });
    await page.goto(`/logs?project=${projectID}&service=checkout`);
    await expect(page.getByRole("button", { name: "Clear filters", exact: true })).toBeVisible();
    const clipped = await page.locator(".log-toolbar-bar").evaluate((toolbar) => {
      const bounds = toolbar.getBoundingClientRect();
      return [...toolbar.querySelectorAll('input:not([type="hidden"]), button')].some((control) => {
        const box = control.getBoundingClientRect();
        return box.width === 0 || box.left < bounds.left - 1 || box.right > bounds.right + 1;
      });
    });
    expect(clipped).toBe(false);
  }

  await page.setViewportSize({ width: 1024, height: 780 });
  await page.goto(`/settings?project=${projectID}`);
  const add = page.getByRole("button", { name: "Add environment", exact: true });
  await expect(add).toBeVisible();
  expect(await add.evaluate((button) => {
    const form = button.closest("form")!.getBoundingClientRect();
    const box = button.getBoundingClientRect();
    return box.left >= form.left - 1 && box.right <= form.right + 1;
  })).toBe(true);

  for (const width of [320, 768]) {
    await page.setViewportSize({ width, height: 780 });
    await page.goto(`/topology?project=${projectID}`);
    await page.getByRole("button", { name: "Graph", exact: true }).click();
    await page.getByRole("button", { name: "SERVICE payments", exact: true }).click();
    const graph = page.getByRole("region", { name: "Dependency graph" });
    await expect(graph.getByText(/2 nodes.*1 relationship/)).toBeVisible();
    const drawer = page.getByRole("complementary", { name: "Details for payments" });
    await drawer.getByRole("link", { name: "Trace ↗", exact: true }).click({ trial: true });
    expect(await graph.locator("header").first().evaluate((header) => header.scrollWidth - header.clientWidth)).toBeLessThanOrEqual(1);
    expect(await drawer.evaluate((panel) => panel.scrollWidth - panel.clientWidth)).toBeLessThanOrEqual(1);
    expect(await drawer.evaluate((panel) => panel.scrollHeight - panel.clientHeight)).toBeLessThanOrEqual(1);
  }
});

for (const theme of ["Light", "Dark"] as const) {
  for (const width of [1440, 390]) {
    test(`keeps Tailwind layouts usable across console pages at ${width}px in ${theme} theme`, async ({ page }, testInfo) => {
      await mockConsole(page);
      await page.setViewportSize({ width, height: 1000 });
      await page.goto(`/overview?project=${projectID}`);
      await chooseTheme(page, theme);
      const routes = [
        ["overview", "Telemetry overview"], ["performance", "Performance"], ["logs", "Log explorer"],
        ["topology", "Service and dependency topology"], ["settings", "Settings"], ["incidents", "Incidents"],
        ["flow", "From signal to answer,"],
      ];
      for (const [route, title] of routes) {
        await page.goto(`/${route}?project=${projectID}`);
        await expect(page.getByRole("heading", { level: 1 })).toContainText(title);
        await expect(page.locator(".loading")).toHaveCount(0);
        expect(await page.evaluate(() => document.body.scrollWidth)).toBeLessThanOrEqual(width);
        const content = await page.locator("#content").boundingBox();
        expect(content!.x + content!.width).toBeLessThanOrEqual(width + 1);
        if (route === "settings") {
          const name = page.getByPlaceholder("e.g. Checkout", { exact: true });
          await expect(name).toHaveCSS("border-top-width", "1px");
          await expect(page.getByRole("button", { name: "Create project", exact: true })).not.toHaveCSS("background-image", "none");
          for (const tab of ["Ingest tokens", "Webhooks", "Members", "Audit", "Projects"]) {
            await page.getByRole("button", { name: tab, exact: true }).click();
            await expect(page.locator(".loading")).toHaveCount(0);
            await expect(page.getByRole("alert")).toHaveCount(0);
          }
        }
        if (route === "performance") {
          await expect(page.getByRole("img", { name: "Observed request rate over time" })).toBeVisible();
          await page.getByRole("tab", { name: /Endpoints/ }).click();
          await expect(page.getByText("GET /orders", { exact: true })).toBeVisible();
          await expect(page.getByRole("tabpanel", { name: "Overview" })).toHaveCount(0);
          await page.getByRole("tab", { name: "Overview", exact: true }).click();
        }
        if (route === "topology") {
          await page.getByRole("button", { name: "Graph", exact: true }).click();
          await page.getByRole("button", { name: "SERVICE payments", exact: true }).click();
          const selected = page.getByRole("button", { name: "SERVICE payments", exact: true });
          await expect(selected.locator(".graph-card")).toHaveCSS("stroke-width", "2px");
        }
        await page.screenshot({ path: testInfo.outputPath(`${route}-${width}-${theme.toLowerCase()}.png`), animations: "disabled" });
      }
    });
  }
}

test("keeps sign-in, registration and reduced-motion forms styled without component CSS", async ({ page }, testInfo) => {
  await page.route("**/api/v1/auth/status", (route) => route.fulfill({ contentType: "application/json", body: JSON.stringify({ registration_open: true }) }));
  await page.setViewportSize({ width: 390, height: 844 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Welcome to TracePilot" })).toBeVisible();
  await expect(page.locator(".welcome-signal .signal-flow").first()).toHaveCSS("animation-name", "none");
  for (const theme of ["Light", "Dark"]) {
    await page.getByRole("button", { name: `${theme} theme` }).click();
    await expect(page.getByLabel("Email", { exact: true })).toHaveCSS("border-top-width", "1px");
    await page.getByRole("button", { name: "Register", exact: true }).click();
    await expect(page.getByLabel("Display name", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Create account", exact: true })).not.toHaveCSS("background-image", "none");
    expect(await page.evaluate(() => document.body.scrollWidth)).toBeLessThanOrEqual(390);
    await page.screenshot({ path: testInfo.outputPath(`register-${theme.toLowerCase()}.png`), animations: "disabled" });
    await page.getByRole("button", { name: "Sign in", exact: true }).first().click();
  }
});
