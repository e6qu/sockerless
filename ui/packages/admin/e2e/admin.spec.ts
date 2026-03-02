import { test, expect } from "@playwright/test";

test.describe("Admin Dashboard", () => {
  test("redirects / to /ui/", async ({ page }) => {
    const response = await page.goto("/");
    // Should have followed the redirect to /ui/
    expect(page.url()).toContain("/ui/");
    expect(response?.status()).toBe(200);
  });

  test("dashboard page renders system overview", async ({ page }) => {
    await page.goto("/ui/");
    // Wait for the overview data to load
    await expect(page.getByText("System Overview")).toBeVisible();
    // KPI cards should be present
    await expect(page.getByText("Components Up")).toBeVisible();
    await expect(page.getByText("Components Down")).toBeVisible();
    await expect(page.getByText("Total Backends")).toBeVisible();
    await expect(page.getByText("Total Containers")).toBeVisible();
  });

  test("dashboard shows component health cards", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByText("Component Health")).toBeVisible();
    // The mock backend is registered as "memory"
    await expect(page.getByText("memory")).toBeVisible();
  });

  test("sidebar has all nav links", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("link", { name: "Dashboard" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Components" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Containers" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Resources" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Metrics" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Contexts" })).toBeVisible();
  });

  test("title shows Sockerless Admin", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByText("Sockerless Admin")).toBeVisible();
  });
});

test.describe("Components Page", () => {
  test("renders component table", async ({ page }) => {
    await page.goto("/ui/");
    await page.getByRole("link", { name: "Components" }).click();
    await expect(page.getByRole("heading", { name: "Components" })).toBeVisible();
    // Table headers
    await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Type" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Health" })).toBeVisible();
    // The mock backend component
    await expect(page.getByRole("cell", { name: "memory" })).toBeVisible();
    await expect(page.getByRole("cell", { name: "backend" })).toBeVisible();
  });

  test("navigates to component detail on row click", async ({ page }) => {
    await page.goto("/ui/components");
    await expect(page.getByRole("cell", { name: "memory" })).toBeVisible();
    await page.getByRole("cell", { name: "memory" }).click();
    await expect(page.url()).toContain("/ui/components/memory");
    await expect(page.getByRole("heading", { name: "memory" })).toBeVisible();
  });
});

test.describe("Component Detail Page", () => {
  test("shows component info and reload button", async ({ page }) => {
    await page.goto("/ui/components/memory");
    // Component name heading
    await expect(page.getByRole("heading", { name: "memory" })).toBeVisible();
    // MetricsCard with Type=backend
    await expect(page.getByText("Type", { exact: true })).toBeVisible();
    // Reload button for backends
    await expect(page.getByRole("button", { name: "Reload" })).toBeVisible();
  });

  test("shows proxied status data", async ({ page }) => {
    await page.goto("/ui/components/memory");
    // Wait for status data to load — the proxied status response should show
    await expect(page.getByRole("heading", { name: "Status" })).toBeVisible();
    await expect(page.getByText("mock-001")).toBeVisible();
  });

  test("shows proxied metrics data", async ({ page }) => {
    await page.goto("/ui/components/memory");
    await expect(page.getByRole("heading", { name: "Metrics" })).toBeVisible();
    // The mock metrics response has goroutines
    await expect(page.getByText("goroutines")).toBeVisible();
  });

  test("reload button triggers POST", async ({ page }) => {
    await page.goto("/ui/components/memory");
    const reloadBtn = page.getByRole("button", { name: "Reload" });
    await expect(reloadBtn).toBeVisible();
    // Click reload and verify it doesn't error (button should return to "Reload" state)
    await reloadBtn.click();
    await expect(reloadBtn).toBeVisible();
    await expect(reloadBtn).not.toBeDisabled();
  });
});

test.describe("Containers Page", () => {
  test("renders aggregated containers", async ({ page }) => {
    await page.goto("/ui/containers");
    await expect(page.getByText("Containers")).toBeVisible();
    // Table headers
    await expect(page.getByRole("columnheader", { name: "Backend" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Image" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "State" })).toBeVisible();
    // Mock container data
    await expect(page.getByRole("cell", { name: "my-web-app" })).toBeVisible();
    await expect(page.getByRole("cell", { name: "nginx:latest" })).toBeVisible();
    // Container count in heading
    await expect(page.getByText("Containers (2)")).toBeVisible();
  });

  test("container filter works", async ({ page }) => {
    await page.goto("/ui/containers");
    await expect(page.getByRole("cell", { name: "my-web-app" })).toBeVisible();
    await expect(page.getByRole("cell", { name: "my-worker" })).toBeVisible();
    // Type in filter
    await page.getByPlaceholder("Filter containers...").fill("worker");
    // Only the worker should be visible
    await expect(page.getByRole("cell", { name: "my-worker" })).toBeVisible();
    await expect(page.getByRole("cell", { name: "my-web-app" })).not.toBeVisible();
  });
});

test.describe("Resources Page", () => {
  test("renders aggregated resources", async ({ page }) => {
    await page.goto("/ui/resources");
    await expect(page.getByText("Cloud Resources")).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Backend" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Type" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Resource ID" })).toBeVisible();
    // Mock resource data
    await expect(page.getByRole("cell", { name: "res-001" })).toBeVisible();
  });
});

test.describe("Metrics Page", () => {
  test("renders per-component metrics panels", async ({ page }) => {
    await page.goto("/ui/metrics");
    await expect(page.getByRole("heading", { name: "Metrics" })).toBeVisible();
    // The memory component panel should show
    await expect(page.getByText("memory")).toBeVisible();
  });
});

test.describe("Contexts Page", () => {
  test("renders contexts table (empty)", async ({ page }) => {
    await page.goto("/ui/contexts");
    await expect(page.getByText("CLI Contexts")).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Backend", exact: true })).toBeVisible();
  });
});

test.describe("Navigation", () => {
  test("navigates between all pages via sidebar", async ({ page }) => {
    await page.goto("/ui/");

    // Dashboard -> Components
    await page.getByRole("link", { name: "Components" }).click();
    await expect(page.url()).toContain("/ui/components");
    await expect(page.getByRole("heading", { name: "Components" })).toBeVisible();

    // Components -> Containers
    await page.getByRole("link", { name: "Containers" }).click();
    await expect(page.url()).toContain("/ui/containers");
    await expect(page.getByText("Containers")).toBeVisible();

    // Containers -> Resources
    await page.getByRole("link", { name: "Resources" }).click();
    await expect(page.url()).toContain("/ui/resources");
    await expect(page.getByText("Cloud Resources")).toBeVisible();

    // Resources -> Metrics
    await page.getByRole("link", { name: "Metrics" }).click();
    await expect(page.url()).toContain("/ui/metrics");
    await expect(page.getByRole("heading", { name: "Metrics" })).toBeVisible();

    // Metrics -> Contexts
    await page.getByRole("link", { name: "Contexts" }).click();
    await expect(page.url()).toContain("/ui/contexts");
    await expect(page.getByText("CLI Contexts")).toBeVisible();

    // Contexts -> Dashboard
    await page.getByRole("link", { name: "Dashboard" }).click();
    await expect(page.url()).toContain("/ui/");
    await expect(page.getByText("System Overview")).toBeVisible();
  });
});
