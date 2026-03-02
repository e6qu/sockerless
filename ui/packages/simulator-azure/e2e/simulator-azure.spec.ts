import { test, expect } from "@playwright/test";

test.describe("Azure Simulator SPA", () => {
  test("redirects / to /ui/", async ({ page }) => {
    const response = await page.goto("/");
    expect(page.url()).toContain("/ui/");
    expect(response?.status()).toBe(200);
  });

  test("renders Azure Simulator title in sidebar", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.locator("h1", { hasText: "Azure Simulator" })).toBeVisible();
  });

  test("sidebar has all 6 nav links", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("link", { name: "Overview" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Container Apps" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Functions" })).toBeVisible();
    await expect(page.getByRole("link", { name: "ACR" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Storage" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Monitor" })).toBeVisible();
  });
});

test.describe("Overview Page", () => {
  test("renders heading and status", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("heading", { name: "Azure Simulator" })).toBeVisible();
  });
});

test.describe("Container Apps Jobs Page", () => {
  test("renders heading and table columns", async ({ page }) => {
    await page.goto("/ui/container-apps");
    await expect(page.getByRole("heading", { name: "Container Apps Jobs" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Location" })).toBeVisible();
  });
});

test.describe("Azure Functions Page", () => {
  test("renders heading and table columns", async ({ page }) => {
    await page.goto("/ui/functions");
    await expect(page.getByRole("heading", { name: "Azure Functions" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Kind" })).toBeVisible();
  });
});

test.describe("ACR Registries Page", () => {
  test("renders heading and table columns", async ({ page }) => {
    await page.goto("/ui/acr");
    await expect(page.getByRole("heading", { name: "ACR Registries" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Location" })).toBeVisible();
  });
});

test.describe("Storage Accounts Page", () => {
  test("renders heading and table columns", async ({ page }) => {
    await page.goto("/ui/storage");
    await expect(page.getByRole("heading", { name: "Storage Accounts" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Kind" })).toBeVisible();
  });
});

test.describe("Azure Monitor Logs Page", () => {
  test("renders heading and table columns", async ({ page }) => {
    await page.goto("/ui/monitor");
    await expect(page.getByRole("heading", { name: "Azure Monitor Logs" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Time" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Source" })).toBeVisible();
  });
});

test.describe("Navigation", () => {
  test("navigates between all pages via sidebar", async ({ page }) => {
    await page.goto("/ui/");

    await page.getByRole("link", { name: "Container Apps" }).click();
    await expect(page.url()).toContain("/ui/container-apps");
    await expect(page.getByRole("heading", { name: "Container Apps Jobs" })).toBeVisible();

    await page.getByRole("link", { name: "Functions" }).click();
    await expect(page.url()).toContain("/ui/functions");
    await expect(page.getByRole("heading", { name: "Azure Functions" })).toBeVisible();

    await page.getByRole("link", { name: "ACR" }).click();
    await expect(page.url()).toContain("/ui/acr");
    await expect(page.getByRole("heading", { name: "ACR Registries" })).toBeVisible();

    await page.getByRole("link", { name: "Storage" }).click();
    await expect(page.url()).toContain("/ui/storage");
    await expect(page.getByRole("heading", { name: "Storage Accounts" })).toBeVisible();

    await page.getByRole("link", { name: "Monitor" }).click();
    await expect(page.url()).toContain("/ui/monitor");
    await expect(page.getByRole("heading", { name: "Azure Monitor Logs" })).toBeVisible();

    await page.getByRole("link", { name: "Overview" }).click();
    await expect(page.url()).toContain("/ui/");
    await expect(page.getByRole("heading", { name: "Azure Simulator" })).toBeVisible();
  });
});
