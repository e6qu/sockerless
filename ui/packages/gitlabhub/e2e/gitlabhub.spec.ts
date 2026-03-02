import { test, expect } from "@playwright/test";

test.describe("gitlabhub Dashboard", () => {
  test("redirects / to /ui/", async ({ page }) => {
    const response = await page.goto("/");
    expect(page.url()).toContain("/ui/");
    expect(response?.status()).toBe(200);
  });

  test("overview page renders title", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByText("gitlabhub")).toBeVisible();
    await expect(page.getByText("Overview")).toBeVisible();
  });

  test("sidebar has all nav links", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("link", { name: "Overview" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Pipelines" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Runners" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Projects" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Metrics" })).toBeVisible();
  });

  test("overview shows metrics cards", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByText("Active Pipelines")).toBeVisible();
    await expect(page.getByText("Registered Runners")).toBeVisible();
    await expect(page.getByText("Submissions")).toBeVisible();
  });
});

test.describe("Navigation", () => {
  test("navigates between pages via sidebar", async ({ page }) => {
    await page.goto("/ui/");

    // Overview -> Pipelines
    await page.getByRole("link", { name: "Pipelines" }).click();
    await expect(page.url()).toContain("/ui/pipelines");
    await expect(page.getByText("Pipelines")).toBeVisible();

    // Pipelines -> Runners
    await page.getByRole("link", { name: "Runners" }).click();
    await expect(page.url()).toContain("/ui/runners");
    await expect(page.getByText("Runners")).toBeVisible();

    // Runners -> Projects
    await page.getByRole("link", { name: "Projects" }).click();
    await expect(page.url()).toContain("/ui/projects");
    await expect(page.getByText("Projects")).toBeVisible();

    // Projects -> Metrics
    await page.getByRole("link", { name: "Metrics" }).click();
    await expect(page.url()).toContain("/ui/metrics");
    await expect(page.getByRole("heading", { name: "Metrics" })).toBeVisible();

    // Metrics -> Overview
    await page.getByRole("link", { name: "Overview" }).click();
    await expect(page.url()).toContain("/ui/");
    await expect(page.getByText("Overview")).toBeVisible();
  });
});
