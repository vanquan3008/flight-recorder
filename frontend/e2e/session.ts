import { expect, type Page } from "@playwright/test";

const email = process.env.TRACEPILOT_E2E_EMAIL || "admin@tracepilot.local";
const password = process.env.TRACEPILOT_E2E_PASSWORD || "admin-password-123";

/**
 * Signs in as the development account created by `tracepilot seed`. A different account is registered
 * first when it does not exist. Accounts without a project create one through the console.
 */
export async function signIn(page: Page, who = { email, password }) {
  await page.request.post("/api/v1/auth/register", { data: { email: who.email, display_name: "E2E", password: who.password }, headers: { "X-TracePilot-Refresh": "1" } });
  await page.goto("/");
  await page.getByLabel("Email").fill(who.email);
  await page.getByLabel("Password").fill(who.password);
  const login = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/v1/auth/login" && response.request().method() === "POST");
  await page.getByRole("button", { name: /^Sign in/ }).last().click();
  await expect(page.getByRole("button", { name: /Sign out/ })).toBeVisible();
  const session = await (await login).json();
  if (session.memberships.length === 0) {
    await expect(page.getByRole("heading", { name: "Create your first project" })).toBeVisible();
    await page.getByLabel("Name", { exact: true }).fill(`E2E ${crypto.randomUUID()}`);
    await page.getByRole("button", { name: "Create project", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Create your first project" })).toBeHidden();
  }
}
