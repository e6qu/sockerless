import { test, expect } from "@playwright/test";

test.describe("Admin application shell", () => {
  test("renders a responsive light and dark shell", async ({ page }) => {
    await page.goto("/ui/");
    await expect((await page.request.get("/ui/favicon.svg")).status()).toBe(200);
    const shell = page.locator(".sl-shell");
    const sidebar = page.getByRole("complementary");
    const skipLink = page.getByRole("link", { name: "Skip to main content" });
    await expect(shell).toHaveCSS("display", "grid");
    await expect(sidebar).toHaveCSS("border-radius", "20px");
    await expect(skipLink).toHaveCSS("width", "1px");
    await expect(skipLink).toHaveCSS("overflow", "hidden");
    await skipLink.focus();
    await expect(skipLink).toBeVisible();
    await expect(skipLink).not.toHaveCSS("width", "1px");

    const darkToggle = page.getByRole("button", { name: "Switch to dark theme" });
    await darkToggle.click();
    await expect(page.locator("html")).toHaveClass(/dark/);
    await page.getByRole("button", { name: "Switch to light theme" }).click();
    await expect(page.locator("html")).not.toHaveClass(/dark/);

    await page.setViewportSize({ width: 700, height: 900 });
    await expect(sidebar).toHaveCSS("position", "static");
  });

  test("redirects the server root to the Admin UI", async ({ page }) => {
    const response = await page.goto("/");
    expect(page.url()).toContain("/ui/");
    expect(response?.status()).toBe(200);
  });

  test("shows the complete current navigation", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("heading", { name: "Admin" })).toBeVisible();
    for (const name of [
      "Dashboard",
      "Topology",
      "Components",
      "Processes",
      "Containers",
      "Cleanup",
      "Metrics",
      "Contexts",
    ]) {
      await expect(page.getByRole("link", { name })).toBeVisible();
    }
  });

  test("serves the public signed-out landing without restarting sign-in", async ({ page }) => {
    const response = await page.goto("/auth/signed-out");
    expect(response?.status()).toBe(200);
    await expect(page).toHaveURL(/\/auth\/signed-out$/);
    await expect(page.getByRole("heading", { name: "Signed out of Sockerless Admin" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Sign in with Shauth" })).toHaveAttribute("href", "/auth/shauth");

    await page.emulateMedia({ colorScheme: "dark" });
    await expect(page.locator("body")).toHaveCSS("background-color", "rgb(22, 12, 9)");
  });
});

test.describe("Dashboard", () => {
  test("renders real component health and overview data", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("heading", { name: "System overview" })).toBeVisible();
    const main = page.getByRole("main");
    for (const label of ["Components up", "Components down", "Backends", "Containers"]) {
      await expect(main.getByText(label, { exact: true })).toBeVisible();
    }
    await expect(page.getByText("Component health", { exact: true })).toBeVisible();
    await expect(page.getByText("docker", { exact: true })).toBeVisible();
  });
});

test.describe("Components", () => {
  test("renders the component table and detail data", async ({ page }) => {
    await page.goto("/ui/components");
    await expect(page.getByRole("heading", { name: "Components" })).toBeVisible();
    for (const name of ["Name", "Type", "Health"]) {
      await expect(page.getByRole("columnheader", { name })).toBeVisible();
    }
    await page.getByRole("cell", { name: "docker" }).click();
    await expect(page).toHaveURL(/\/ui\/components\/docker$/);
    await expect(page.getByRole("heading", { name: "docker" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Reload" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Status" })).toBeVisible();
    await expect(page.getByText("sockerless-docker")).toBeVisible();
    await expect(page.getByRole("heading", { name: "Metrics" })).toBeVisible();
    await expect(page.getByText("goroutines")).toBeVisible();
  });

  test("reloads the real registered backend", async ({ page }) => {
    await page.goto("/ui/components/docker");
    const reload = page.getByRole("button", { name: "Reload" });
    await reload.click();
    await expect(reload).toBeEnabled();
  });
});

test.describe("Containers", () => {
  test("renders the real aggregated container surface", async ({ page }) => {
    await page.goto("/ui/containers");
    await expect(page.getByRole("heading", { name: "Containers" })).toBeVisible();
    for (const name of ["Backend", "Name", "Image", "State"]) {
      await expect(page.getByRole("columnheader", { name })).toBeVisible();
    }
    await expect(page.getByRole("textbox", { name: "Filter table" })).toBeVisible();
    await expect(page.getByText("No containers found across any backend.")).toBeVisible();
  });
});

test.describe("Operational pages", () => {
  test("renders topology, processes, cleanup, metrics, and contexts", async ({ page }) => {
    const pages: Array<[string, string]> = [
      ["Topology", "Topology"],
      ["Processes", "Managed processes"],
      ["Cleanup", "Stale-resource sweep"],
      ["Metrics", "Per-component metrics"],
      ["Contexts", "CLI contexts"],
    ];

    await page.goto("/ui/");
    for (const [link, heading] of pages) {
      await page.getByRole("link", { name: link }).click();
      await expect(page.getByRole("heading", { name: heading })).toBeVisible();
    }
    await page.getByRole("link", { name: "Dashboard" }).click();
    await expect(page.getByRole("heading", { name: "System overview" })).toBeVisible();
  });
});
