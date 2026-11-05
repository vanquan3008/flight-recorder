import { expect, test } from "@playwright/test";
import { signIn } from "./session";

test("signs in and sees the empty telemetry state without demo data", async ({ page }) => {
  await signIn(page);
  await expect(page.getByRole("heading", { name: "Get your first trace" })).toBeVisible();

  await page.getByRole("link", { name: "Trace explorer" }).click();
  await expect(page.getByText("No traces match those filters.")).toBeVisible();
});
