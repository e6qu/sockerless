import { test, expect } from "@playwright/test";

test.describe("AWS Simulator SPA", () => {
  test("redirects / to /ui/", async ({ page }) => {
    const response = await page.goto("/");
    expect(page.url()).toContain("/ui/");
    expect(response?.status()).toBe(200);
  });

  test("renders AWS Simulator title in sidebar", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.locator("h1", { hasText: "AWS Simulator" })).toBeVisible();
  });

  test("sidebar has all 6 nav links", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("link", { name: "Overview" })).toBeVisible();
    await expect(page.getByRole("link", { name: "ECS Tasks" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Lambda" })).toBeVisible();
    await expect(page.getByRole("link", { name: "ECR" })).toBeVisible();
    await expect(page.getByRole("link", { name: "S3" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Logs" })).toBeVisible();
  });
});

test.describe("Overview Page", () => {
  test("renders heading and status", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("heading", { name: "AWS Simulator" })).toBeVisible();
  });
});

test.describe("ECS Tasks Page", () => {
  test("renders heading and table columns", async ({ page }) => {
    await page.goto("/ui/ecs");
    await expect(page.getByRole("heading", { name: "ECS Tasks" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Task ARN" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Status" })).toBeVisible();
  });
});

test.describe("Lambda Functions Page", () => {
  test("renders heading and table columns", async ({ page }) => {
    await page.goto("/ui/lambda");
    await expect(page.getByRole("heading", { name: "Lambda Functions" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Runtime" })).toBeVisible();
  });
});

test.describe("ECR Repositories Page", () => {
  test("renders heading and table columns", async ({ page }) => {
    await page.goto("/ui/ecr");
    await expect(page.getByRole("heading", { name: "ECR Repositories" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "URI" })).toBeVisible();
  });
});

test.describe("S3 Buckets Page", () => {
  test("renders heading and table columns", async ({ page }) => {
    await page.goto("/ui/s3");
    await expect(page.getByRole("heading", { name: "S3 Buckets" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
  });
});

test.describe("CloudWatch Log Groups Page", () => {
  test("renders heading and table columns", async ({ page }) => {
    await page.goto("/ui/logs");
    await expect(page.getByRole("heading", { name: "CloudWatch Log Groups" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Retention (days)" })).toBeVisible();
  });
});

test.describe("Navigation", () => {
  test("navigates between all pages via sidebar", async ({ page }) => {
    await page.goto("/ui/");

    await page.getByRole("link", { name: "ECS Tasks" }).click();
    await expect(page.url()).toContain("/ui/ecs");
    await expect(page.getByRole("heading", { name: "ECS Tasks" })).toBeVisible();

    await page.getByRole("link", { name: "Lambda" }).click();
    await expect(page.url()).toContain("/ui/lambda");
    await expect(page.getByRole("heading", { name: "Lambda Functions" })).toBeVisible();

    await page.getByRole("link", { name: "ECR" }).click();
    await expect(page.url()).toContain("/ui/ecr");
    await expect(page.getByRole("heading", { name: "ECR Repositories" })).toBeVisible();

    await page.getByRole("link", { name: "S3" }).click();
    await expect(page.url()).toContain("/ui/s3");
    await expect(page.getByRole("heading", { name: "S3 Buckets" })).toBeVisible();

    await page.getByRole("link", { name: "Logs" }).click();
    await expect(page.url()).toContain("/ui/logs");
    await expect(page.getByRole("heading", { name: "CloudWatch Log Groups" })).toBeVisible();

    await page.getByRole("link", { name: "Overview" }).click();
    await expect(page.url()).toContain("/ui/");
    await expect(page.getByRole("heading", { name: "AWS Simulator" })).toBeVisible();
  });
});
