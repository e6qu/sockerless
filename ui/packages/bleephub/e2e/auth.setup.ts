import { test as setup, expect } from "@playwright/test";

const authFile = ".auth/storage-state.json";

setup("authenticate", async ({ page }) => {
  await page.goto("/ui/login");
  await page.getByLabel(/token/i).fill("bleephub-admin-token-00000000000000000000");
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL("/ui/");
  await page.context().storageState({ path: authFile });
});
