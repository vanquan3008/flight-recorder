import { expect, type APIRequestContext, type Page } from "@playwright/test";

export const admin = {
  email: process.env.TRACEPILOT_E2E_EMAIL || "admin@tracepilot.test",
  password: process.env.TRACEPILOT_E2E_PASSWORD || "admin-password-test-only-123",
};

async function accessToken(request: APIRequestContext, who: { email: string; password: string }) {
  const response = await request.post("/api/v1/auth/login", { data: who });
  return response.ok() ? ((await response.json()).access_token as string) : "";
}

// A fresh account per test keeps its project empty and gives each login its own rate-limit key.
async function createTestAccount(request: APIRequestContext, who: typeof admin) {
  const temporary = "e2e-temporary-passphrase";
  const created = await request.post("/api/v1/users", {
    data: { email: who.email, display_name: "E2E", password: temporary },
    headers: { Authorization: `Bearer ${await accessToken(request, admin)}` },
  });
  expect(created.status(), "the super administrator creates the test account").toBe(201);
  const changed = await request.post("/api/v1/auth/password", {
    data: { current_password: temporary, new_password: who.password },
    headers: { Authorization: `Bearer ${await accessToken(request, { email: who.email, password: temporary })}` },
  });
  expect(changed.ok(), "the test account replaces its temporary password").toBeTruthy();
}

export async function signIn(page: Page) {
  const who = { email: `e2e-${crypto.randomUUID()}@tracepilot.test`, password: "e2e-account-passphrase" };
  await createTestAccount(page.request, who);
  await page.goto("/");
  await page.getByLabel("Email").fill(who.email);
  await page.getByLabel("Password", { exact: true }).fill(who.password);
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
