import { expect, test } from "@playwright/test";

const checkoutTraceID = "33333333333333333333333333333333";

test("connects, finds the seeded failed trace, and opens its correlated log", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("API key").fill(process.env.TRACEPILOT_E2E_API_KEY || "tracepilot-development-bootstrap-key-000000");
  await page.getByRole("button", { name: "Connect" }).click();
  await expect(page.getByRole("heading", { name: "Get your first trace" })).toBeVisible();
  await expect(page.getByText("First trace received.")).toBeVisible();

  await page.getByRole("link", { name: "Trace explorer" }).click();
  const trace = page.locator("tr").filter({ hasText: checkoutTraceID });
  await trace.getByRole("link", { name: "Inspect" }).click();
  await expect(page.getByRole("heading", { name: "Trace detail" })).toBeVisible();
  await expect(page.getByText("checkout returned 500")).toBeVisible();
});
