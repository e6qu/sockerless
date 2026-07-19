import { test, expect } from "@playwright/test";

test.describe("GCP Simulator SPA", () => {
  test("uses the shared responsive application shell", async ({ page }) => {
    await page.goto("/ui/");
    await expect((await page.request.get("/ui/favicon.svg")).status()).toBe(200);
    const shell = page.locator(".sl-shell");
    const sidebar = page.getByRole("complementary");
    await expect(shell).toHaveCSS("display", "grid");
    await expect(sidebar).toHaveCSS("border-radius", "20px");
    await page.setViewportSize({ width: 700, height: 900 });
    await expect(sidebar).toHaveCSS("position", "static");
  });

  test("redirects / to /ui/", async ({ page }) => {
    const response = await page.goto("/");
    expect(page.url()).toContain("/ui/");
    expect(response?.status()).toBe(200);
  });

  test("renders GCP Simulator title in sidebar", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.locator("h1", { hasText: "GCP Simulator" })).toBeVisible();
  });

  test("sidebar has all 6 nav links", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("link", { name: "Overview" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Cloud Run Jobs" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Functions" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Artifact Registry" })).toBeVisible();
    await expect(page.getByRole("link", { name: "GCS" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Logging" })).toBeVisible();
  });
});

test.describe("Overview Page", () => {
  test("renders heading and status", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
  });
});

test.describe("Cloud Run Jobs Page", () => {
  test("renders heading and table columns", async ({ page }) => {
    await page.goto("/ui/cloudrun");
    await expect(page.getByRole("heading", { name: "Cloud Run Jobs" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Executions" })).toBeVisible();
  });
});

test.describe("Cloud Functions Page", () => {
  test("renders heading and table columns", async ({ page }) => {
    await page.goto("/ui/functions");
    await expect(page.getByRole("heading", { name: "Cloud Functions" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "State" })).toBeVisible();
  });
});

test.describe("Artifact Registry Page", () => {
  test("renders heading and table columns", async ({ page }) => {
    await page.goto("/ui/ar");
    await expect(page.getByRole("heading", { name: "Artifact Registry" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Format" })).toBeVisible();
  });
});

test.describe("GCS Buckets Page", () => {
  test("renders heading and table", async ({ page }) => {
    await page.goto("/ui/gcs");
    await expect(page.getByRole("heading", { name: "GCS Buckets" })).toBeVisible();
    await expect(page.locator("table")).toBeVisible();
  });
});

test.describe("Cloud Logging Page", () => {
  test("renders heading and table columns", async ({ page }) => {
    await page.goto("/ui/logging");
    await expect(page.getByRole("heading", { name: "Cloud Logging" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Timestamp" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Severity" })).toBeVisible();
  });
});

test.describe("Navigation", () => {
  test("navigates between all pages via sidebar", async ({ page }) => {
    await page.goto("/ui/");

    await page.getByRole("link", { name: "Cloud Run Jobs" }).click();
    await expect(page.url()).toContain("/ui/cloudrun");
    await expect(page.getByRole("heading", { name: "Cloud Run Jobs" })).toBeVisible();

    await page.getByRole("link", { name: "Functions" }).click();
    await expect(page.url()).toContain("/ui/functions");
    await expect(page.getByRole("heading", { name: "Cloud Functions" })).toBeVisible();

    await page.getByRole("link", { name: "Artifact Registry" }).click();
    await expect(page.url()).toContain("/ui/ar");
    await expect(page.getByRole("heading", { name: "Artifact Registry" })).toBeVisible();

    await page.getByRole("link", { name: "GCS" }).click();
    await expect(page.url()).toContain("/ui/gcs");
    await expect(page.getByRole("heading", { name: "GCS Buckets" })).toBeVisible();

    await page.getByRole("link", { name: "Logging" }).click();
    await expect(page.url()).toContain("/ui/logging");
    await expect(page.getByRole("heading", { name: "Cloud Logging" })).toBeVisible();

    await page.getByRole("link", { name: "Overview" }).click();
    await expect(page.url()).toContain("/ui/");
    await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
  });
});
