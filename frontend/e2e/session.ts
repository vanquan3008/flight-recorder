import { expect, type Page } from "@playwright/test";

const email = process.env.TRACEPILOT_E2E_EMAIL || "admin@tracepilot.local";
const password = process.env.TRACEPILOT_E2E_PASSWORD || "admin-password-123";

/**
 * Signs in as the test account: the admin that backend/db/seed.sql creates, which owns the seeded default
 * project. A different account is registered first when it does not exist.
 */
export async function signIn(page: Page, who = { email, password }) {
  await page.request.post("/api/v1/auth/register", { data: { email: who.email, display_name: "E2E", password: who.password }, headers: { "X-TracePilot-Refresh": "1" } });
  await page.goto("/");
  await page.getByLabel("Email").fill(who.email);
  await page.getByLabel("Password").fill(who.password);
  await page.getByRole("button", { name: /^Sign in/ }).last().click();
  await expect(page.getByRole("button", { name: /Sign out/ })).toBeVisible();
}
