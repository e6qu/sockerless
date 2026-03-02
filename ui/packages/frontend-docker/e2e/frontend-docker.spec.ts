import { test, expect } from "@playwright/test";

test.describe("Docker Frontend SPA", () => {
  test("redirects / to /ui/", async ({ page }) => {
    const response = await page.goto("/");
    expect(page.url()).toContain("/ui/");
    expect(response?.status()).toBe(200);
  });

  test("renders Docker Frontend title in sidebar", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByText("Docker Frontend")).toBeVisible();
  });

  test("renders Frontend Overview heading", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("heading", { name: "Frontend Overview" })).toBeVisible();
  });

  test("renders KPI cards", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByText("Docker Requests")).toBeVisible();
    await expect(page.getByText("Goroutines")).toBeVisible();
    await expect(page.getByText("Heap")).toBeVisible();
    await expect(page.getByText("Uptime")).toBeVisible();
  });

  test("renders Configuration section", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByText("Docker Address")).toBeVisible();
    await expect(page.getByText("Backend Address")).toBeVisible();
  });

  test("shows health status", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByText("Health:")).toBeVisible();
  });
});
