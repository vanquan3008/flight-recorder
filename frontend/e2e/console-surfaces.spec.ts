import { expect, test, type Page } from "@playwright/test";
import { admin, signIn } from "./session";

async function connect(page: Page) {
  await signIn(page);
  // Pages render their own role="status" loading indicators, so address the connection badge by its name.
  await expect(page.getByRole("status", { name: "Connected" })).toBeVisible();
}

/**
 * Picks a time range on the page titled `heading`. Router navigations commit as transitions, so the previous
 * page (which has the same radios) stays on screen for a moment after a sidebar click: wait for the target page,
 * or the click lands on the old one and then navigates back to it. The radios are controlled by the URL, so
 * the input is also briefly reverted after the click: `.check()` would read that and fail. Click, then wait.
 */
async function chooseRange(page: Page, heading: string, name: string) {
  await expect(page.getByRole("heading", { name: heading, level: 1 })).toBeVisible();
  const range = page.getByRole("radio", { name });
  await range.click({ force: true });
  await expect(range).toBeChecked();
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

test("keeps trace filters on an empty result list", async ({ page }) => {
  await connect(page);
  await page.getByRole("link", { name: "Trace explorer" }).click();
  await page.getByLabel("Filter by service").fill("checkout");
  await page.getByLabel("Filter by status").selectOption("ERROR");
  await page.getByRole("button", { name: "Search traces" }).click();
  await expect(page.getByText("No traces match those filters.")).toBeVisible();
  await expect(page.getByLabel("Filter by service")).toHaveValue("checkout");
  await expect(page.getByLabel("Filter by status")).toHaveValue("ERROR");
  await page.reload();
  await expect(page.getByLabel("Filter by service")).toHaveValue("checkout");
  await expect(page.getByLabel("Filter by status")).toHaveValue("ERROR");
});

test("shows an empty log explorer before telemetry arrives", async ({ page }) => {
  await connect(page);
  await page.getByRole("link", { name: "Log explorer" }).click();
  await chooseRange(page, "Log explorer", "Last 24 hours");
  await expect(page.getByText("No logs match these filters in the selected time range.")).toBeVisible();
});

test("shows an empty service topology before telemetry arrives", async ({ page }) => {
  await connect(page);
  await page.getByRole("link", { name: "Service topology" }).click();
  await chooseRange(page, "Service and dependency topology", "Last 24 hours");
  await expect(page.getByText("No services were observed in the selected time range.")).toBeVisible();
});

test("manages ingest tokens with a one-time secret and a confirmed revoke", async ({ page }) => {
  await connect(page);
  await page.getByRole("link", { name: "Settings" }).click();
  await page.getByRole("button", { name: "Ingest tokens" }).click();
  // The tab is URL state and commits as a transition; the Projects tab also has "Name" fields until it does.
  await expect(page.getByRole("heading", { name: "New ingest token" })).toBeVisible();
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

test("an account the super administrator creates replaces its temporary password, then is offered a first project", async ({ page }) => {
  const stranger = { email: `e2e-${Date.now().toString(36)}@example.test`, temporary: "a-temporary-e2e-passphrase", password: "another-e2e-passphrase" };
  const { access_token: token } = await (await page.request.post("/api/v1/auth/login", { data: admin })).json();
  const created = await page.request.post("/api/v1/users", {
    data: { email: stranger.email, display_name: "Stranger", password: stranger.temporary },
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(created.status()).toBe(201);
  await page.goto("/");
  await page.getByLabel("Email").fill(stranger.email);
  await page.getByLabel("Password", { exact: true }).fill(stranger.temporary);
  await page.getByRole("button", { name: /^Sign in/ }).last().click();
  await page.getByLabel("Temporary password", { exact: true }).fill(stranger.temporary);
  await page.getByLabel("New password", { exact: true }).fill(stranger.password);
  await page.getByLabel("Repeat new password", { exact: true }).fill(stranger.password);
  await page.getByRole("button", { name: "Save password" }).click();
  await expect(page.getByRole("heading", { name: "Create your first project" })).toBeVisible();
});

test("shows setup guidance until a real trace arrives", async ({ page }) => {
  await connect(page);
  await expect(page.getByRole("heading", { name: "Get your first trace" })).toBeVisible();
  await expect(page.getByText("Send a request from your instrumented application, then refresh this page.")).toBeVisible();
});
