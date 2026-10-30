import { expect, test, type Page } from "@playwright/test";

const key = process.env.TRACEPILOT_E2E_API_KEY || "tracepilot-development-bootstrap-key-000000";

async function connect(page: Page) {
  await page.goto("/");
  await page.getByLabel("API key").fill(key);
  await page.getByRole("button", { name: "Connect" }).click();
  await expect(page.getByText("Connected")).toBeVisible();
}

test("keeps the chosen theme across reloads without painting the other one first", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await connect(page);
  await page.getByRole("button", { name: "Light theme" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await page.getByRole("button", { name: "System theme" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
});

test("returns from a trace to the filtered list the operator came from", async ({ page }) => {
  await connect(page);
  await page.getByRole("link", { name: "Trace explorer" }).click();
  await page.getByLabel("Filter by service").fill("checkout");
  await page.getByLabel("Filter by status").fill("ERROR");
  await page.getByRole("button", { name: "Search traces" }).click();
  await page.getByRole("link", { name: "Inspect" }).first().click();
  await expect(page.getByRole("heading", { name: "Trace detail" })).toBeVisible();
  await page.getByRole("link", { name: /Back to traces/ }).click();
  await expect(page.getByLabel("Filter by service")).toHaveValue("checkout");
  await expect(page.getByLabel("Filter by status")).toHaveValue("ERROR");
});

test("reads logs independently and follows one to its trace", async ({ page }) => {
  await connect(page);
  await page.getByRole("link", { name: "Log explorer" }).click();
  await page.getByLabel("Time range").selectOption("24h");
  await expect(page.getByRole("heading", { name: "Log explorer" })).toBeVisible();
  await expect(page.getByRole("row").nth(1)).toBeVisible();
  await page.getByRole("link", { name: "Open trace" }).first().click();
  await expect(page.getByRole("heading", { name: "Trace detail" })).toBeVisible();
  await page.getByRole("link", { name: /Back to logs/ }).click();
  await expect(page.getByRole("heading", { name: "Log explorer" })).toBeVisible();
});

test("shows the dependency graph and opens a node's calls", async ({ page }) => {
  await connect(page);
  await page.getByRole("link", { name: "Service topology" }).click();
  await page.getByLabel("Time range").selectOption("24h");
  const nodes = page.getByRole("button", { name: /^service |^database / });
  await expect(nodes.first()).toBeVisible();
  await nodes.first().click();
  await expect(page.getByRole("heading", { name: "Calls out" })).toBeVisible();
});

test("manages API keys with a one-time secret and a confirmed revoke", async ({ page }) => {
  await connect(page);
  await page.getByRole("link", { name: "Settings" }).click();
  const name = `e2e-${Date.now().toString(36)}`;
  await page.getByLabel("Name").fill(name);
  await page.getByRole("button", { name: "Create key" }).click();
  await expect(page.getByText(/shown only once/)).toBeVisible();
  await page.getByRole("button", { name: "I have stored it" }).click();
  await expect(page.getByText(/shown only once/)).toBeHidden();
  await expect(page.getByRole("cell", { name, exact: true })).toBeVisible();
  await page.getByRole("button", { name: `Revoke ${name}` }).click();
  await page.getByRole("button", { name: "Confirm revoke" }).click();
  await expect(page.getByRole("row", { name: new RegExp(name) }).getByText("REVOKED")).toBeVisible();
});

test("tells a read-only key why settings are unavailable", async ({ page, request }) => {
  const issued = await request.post("/api/v1/api-keys", { headers: { "X-API-Key": key }, data: { name: `ro-${Date.now().toString(36)}`, scopes: ["read"] } });
  expect(issued.ok()).toBeTruthy();
  const readOnly = (await issued.json()).key as string;
  await page.goto("/");
  await page.getByLabel("API key").fill(readOnly);
  await page.getByRole("button", { name: "Connect" }).click();
  await page.getByRole("link", { name: "Settings" }).click();
  await expect(page.getByText("This key cannot open this view")).toBeVisible();
});

test("shows the request flow, time axis and waterfall for a stored trace", async ({ page }) => {
  await connect(page);
  await page.getByRole("link", { name: "Trace explorer" }).click();
  await page.getByRole("link", { name: "Inspect" }).first().click();
  await expect(page.getByRole("heading", { name: "Request flow" })).toBeVisible();
  await expect(page.getByText("Total duration")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Where the time went" })).toBeVisible();
  await expect(page.locator(".wf-axis")).toBeVisible();
  const first = page.locator(".wf-rows .wf-main").first();
  await first.click();
  await expect(page.getByRole("link", { name: "Logs for this span" })).toBeVisible();
});
