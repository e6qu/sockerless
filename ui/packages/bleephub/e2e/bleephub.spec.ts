import { test, expect } from "@playwright/test";

test.describe("bleephub Dashboard", () => {
  test("redirects / to /ui/", async ({ page }) => {
    const response = await page.goto("/");
    expect(page.url()).toContain("/ui/");
    expect(response?.status()).toBe(200);
  });

  test("overview page renders title", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByText("bleephub")).toBeVisible();
    await expect(page.getByText("Overview")).toBeVisible();
  });

  test("sidebar has all nav links", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("link", { name: "Overview" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Workflows" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Runners" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Repos" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Metrics" })).toBeVisible();
  });

  test("overview shows metrics cards", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByText("Active Workflows")).toBeVisible();
    await expect(page.getByText("Connected Runners")).toBeVisible();
    await expect(page.getByText("Submissions")).toBeVisible();
  });
});

test.describe("Navigation", () => {
  test("navigates between pages via sidebar", async ({ page }) => {
    await page.goto("/ui/");

    // Overview -> Workflows
    await page.getByRole("link", { name: "Workflows" }).click();
    await expect(page.url()).toContain("/ui/workflows");
    await expect(page.getByText("Workflows")).toBeVisible();

    // Workflows -> Runners
    await page.getByRole("link", { name: "Runners" }).click();
    await expect(page.url()).toContain("/ui/runners");
    await expect(page.getByText("Runners")).toBeVisible();

    // Runners -> Repos
    await page.getByRole("link", { name: "Repos" }).click();
    await expect(page.url()).toContain("/ui/repos");
    await expect(page.getByText("Repositories")).toBeVisible();

    // Repos -> Metrics
    await page.getByRole("link", { name: "Metrics" }).click();
    await expect(page.url()).toContain("/ui/metrics");
    await expect(page.getByRole("heading", { name: "Metrics" })).toBeVisible();

    // Metrics -> Overview
    await page.getByRole("link", { name: "Overview" }).click();
    await expect(page.url()).toContain("/ui/");
    await expect(page.getByText("Overview")).toBeVisible();
  });
});
