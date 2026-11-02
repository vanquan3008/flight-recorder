import { expect, test, type Page } from "@playwright/test";
import { signIn } from "./session";

async function connect(page: Page) {
  await signIn(page);
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
  await page.getByRole("radio", { name: "Last 24 hours" }).check({ force: true });
  await expect(page.getByRole("heading", { name: "Log explorer" })).toBeVisible();
  await expect(page.getByRole("row").nth(1)).toBeVisible();
  await page.getByRole("link", { name: "Open trace" }).first().click();
  await expect(page.getByRole("heading", { name: "Trace detail" })).toBeVisible();
  await page.getByRole("link", { name: /Back to logs/ }).click();
  await expect(page.getByRole("heading", { name: "Log explorer" })).toBeVisible();
});

test("lists services and shows where one sits in the flow", async ({ page }) => {
  await connect(page);
  await page.getByRole("link", { name: "Service topology" }).click();
  await page.getByRole("radio", { name: "Last 24 hours" }).check({ force: true });
  const rows = page.locator(".service-row");
  await expect(rows.first()).toBeVisible();
  await rows.first().click();
  await expect(page.getByRole("group", { name: /sits in the flow/ })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Recent logs" })).toBeVisible();
});

test("manages ingest tokens with a one-time secret and a confirmed revoke", async ({ page }) => {
  await connect(page);
  await page.getByRole("link", { name: "Settings" }).click();
  await page.getByRole("button", { name: "Ingest tokens" }).click();
  const name = `e2e-${Date.now().toString(36)}`;
  await page.getByLabel("Name").fill(name);
  await page.getByRole("button", { name: "Create token" }).click();
  await expect(page.getByText(/shown only once/)).toBeVisible();
  await page.getByRole("button", { name: "I have stored it" }).click();
  await expect(page.getByText(/shown only once/)).toBeHidden();
  await expect(page.getByRole("cell", { name, exact: true })).toBeVisible();
  await page.getByRole("button", { name: `Revoke ${name}` }).click();
  await page.getByRole("button", { name: "Confirm revoke" }).click();
  await expect(page.getByRole("row", { name: new RegExp(name) }).getByText("REVOKED")).toBeVisible();
});

test("a newly registered person owns nothing yet and is offered a first project", async ({ page }) => {
  const stranger = { email: `e2e-${Date.now().toString(36)}@example.test`, password: "another-e2e-passphrase" };
  await page.request.post("/api/v1/auth/register", { data: { email: stranger.email, display_name: "Stranger", password: stranger.password }, headers: { "X-TracePilot-Refresh": "1" } });
  await page.goto("/");
  await page.getByLabel("Email").fill(stranger.email);
  await page.getByLabel("Password").fill(stranger.password);
  await page.getByRole("button", { name: /^Sign in/ }).last().click();
  await expect(page.getByRole("heading", { name: "Create your first project" })).toBeVisible();
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
