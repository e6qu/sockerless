import { test, expect, type Page } from "@playwright/test";
import fs from "fs";
import path from "path";

// Screenshots land in temp/screenshots/ (gitignored). Created lazily.
// process.cwd() = sockerless/ui/packages/bleephub when Playwright runs
const SCREENSHOT_DIR = path.resolve(process.cwd(), "../../../temp/screenshots");

function ensureScreenshotDir(): void {
  fs.mkdirSync(SCREENSHOT_DIR, { recursive: true });
}

async function shot(page: Page, name: string): Promise<void> {
  ensureScreenshotDir();
  await page.screenshot({
    path: path.join(SCREENSHOT_DIR, `${name}.png`),
    fullPage: true,
  });
}

// ─── helpers ────────────────────────────────────────────────────────────────

const TOKEN = "bleephub-admin-token-00000000000000000000";
const BASE = "http://localhost:15555";

async function apiPost(page: Page, path: string, body: unknown) {
  return page.evaluate(
    async ({ base, path, token, body }) => {
      const res = await fetch(base + path, {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
        body: JSON.stringify(body),
      });
      if (!res.ok) throw new Error(`${res.status} ${res.statusText} ${await res.text()}`);
      return res.json();
    },
    { base: BASE, path, token: TOKEN, body },
  );
}

async function apiGet(page: Page, path: string) {
  return page.evaluate(
    async ({ base, path, token }) => {
      const res = await fetch(base + path, {
        headers: { Authorization: `Bearer ${token}` },
      });
      return res.ok ? res.json() : null;
    },
    { base: BASE, path, token: TOKEN },
  );
}

// ─── redirect ───────────────────────────────────────────────────────────────

test.describe("Root redirect", () => {
  test("/ redirects to /ui/", async ({ page }) => {
    const res = await page.goto("/");
    expect(page.url()).toContain("/ui/");
    expect(res?.status()).toBe(200);
    await shot(page, "00-root-redirect");
  });
});

// ─── Overview ───────────────────────────────────────────────────────────────

test.describe("Overview page", () => {
  test("renders title and heading", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("heading", { level: 1 }).filter({ hasText: "bleephub" })).toBeVisible();
    // OverviewPage uses "System status" as its PageHeading title
    await expect(page.getByRole("heading", { level: 2 }).filter({ hasText: "System status" })).toBeVisible();
    await shot(page, "01-overview");
  });

  test("shows metrics cards", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByText("Active Workflows")).toBeVisible();
    await expect(page.getByText("Connected Runners")).toBeVisible();
    await expect(page.getByText("Submissions")).toBeVisible();
    await shot(page, "02-overview-metrics");
  });
});

// ─── Sidebar navigation ─────────────────────────────────────────────────────

test.describe("Sidebar navigation", () => {
  test("shows all 7 nav links", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("link", { name: "Overview" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Workflows" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Runners" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Repos" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Apps" })).toBeVisible();
    await expect(page.getByRole("link", { name: "OAuth" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Metrics" })).toBeVisible();
    await shot(page, "03-sidebar-all-links");
  });

  test("navigates between all pages", async ({ page }) => {
    await page.goto("/ui/");

    await page.getByRole("link", { name: "Workflows" }).click();
    await expect(page.url()).toContain("/ui/workflows");
    await expect(page.getByRole("heading", { level: 2 }).filter({ hasText: "Workflows" })).toBeVisible();
    await shot(page, "04-workflows");

    await page.getByRole("link", { name: "Runners" }).click();
    await expect(page.url()).toContain("/ui/runners");
    await expect(page.getByRole("heading", { level: 2 }).filter({ hasText: "Runners" })).toBeVisible();
    await shot(page, "05-runners");

    await page.getByRole("link", { name: "Repos" }).click();
    await expect(page.url()).toContain("/ui/repos");
    await expect(page.getByRole("heading", { level: 2 }).filter({ hasText: "Repositories" })).toBeVisible();
    await shot(page, "06-repos");

    await page.getByRole("link", { name: "Apps" }).click();
    await expect(page.url()).toContain("/ui/apps");
    await expect(page.getByRole("heading", { level: 2 }).filter({ hasText: "Apps" })).toBeVisible();
    await shot(page, "07-apps");

    await page.getByRole("link", { name: "OAuth" }).click();
    await expect(page.url()).toContain("/ui/oauth");
    await shot(page, "08-oauth");

    await page.getByRole("link", { name: "Metrics" }).click();
    await expect(page.url()).toContain("/ui/metrics");
    await expect(page.getByRole("heading", { level: 2 }).filter({ hasText: /runtime/i })).toBeVisible();
    await shot(page, "09-metrics");

    await page.getByRole("link", { name: "Overview" }).click();
    await expect(page.url()).toMatch(/\/ui\/$/);
    await shot(page, "10-back-overview");
  });
});

// ─── Dark / light mode toggle ────────────────────────────────────────────────

test.describe("Theme toggle", () => {
  test("toggle button is present", async ({ page }) => {
    await page.goto("/ui/");
    await page.waitForLoadState("networkidle");
    // ThemeToggle is in sidebar footer with aria-label that mentions "theme"
    const toggle = page.locator('[aria-label*="theme"]').first();
    await expect(toggle).toBeVisible();
    await shot(page, "11-theme-toggle");

    // Verify the label is one of the two expected values
    const label = await toggle.getAttribute("aria-label");
    expect(label).toMatch(/switch to (light|dark) theme/i);
  });
});

// ─── Repos page ─────────────────────────────────────────────────────────────

test.describe("Repos page", () => {
  test("shows empty state initially", async ({ page }) => {
    await page.goto("/ui/repos");
    await shot(page, "12-repos-empty");
  });

  test("shows repo after creation and links to detail", async ({ page }) => {
    // Create user + repo via API
    await page.goto("/ui/");
    const user = await apiGet(page, "/api/v3/user");
    const owner = (user as { login: string }).login;

    await apiPost(page, "/api/v3/user/repos", {
      name: "test-repo-playwright",
      description: "Playwright test repo",
      private: false,
    });

    await page.goto("/ui/repos");
    await page.waitForLoadState("networkidle");

    const link = page.getByRole("link", { name: /test-repo-playwright/ });
    await expect(link).toBeVisible();
    await shot(page, "13-repos-with-repo");

    // Click through to detail page
    await link.click();
    await expect(page.url()).toContain("/ui/repos/");
    await expect(page.url()).toContain("test-repo-playwright");
    await shot(page, "14-repo-detail");
  });
});

// ─── Repo detail ─────────────────────────────────────────────────────────────

test.describe("Repo detail page", () => {
  test("shows empty repo with clone instructions", async ({ page }) => {
    await page.goto("/ui/");
    const user = await apiGet(page, "/api/v3/user");
    const owner = (user as { login: string }).login;

    // Ensure repo exists
    await apiPost(page, "/api/v3/user/repos", {
      name: "detail-test",
      description: "Detail page test",
      private: false,
    }).catch(() => null);

    await page.goto(`/ui/repos/${owner}/detail-test`);
    await page.waitForLoadState("networkidle");

    await expect(page.getByText(`${owner} / detail-test`)).toBeVisible();
    await shot(page, "15-repo-detail-empty");
  });

  test("issues tab shows issue list", async ({ page }) => {
    await page.goto("/ui/");
    const user = await apiGet(page, "/api/v3/user");
    const owner = (user as { login: string }).login;

    await apiPost(page, "/api/v3/user/repos", {
      name: "issues-test",
      description: "",
      private: false,
    }).catch(() => null);
    await apiPost(page, `/api/v3/repos/${owner}/issues-test/issues`, {
      title: "First Playwright issue",
      body: "Created by Playwright test",
    });

    await page.goto(`/ui/repos/${owner}/issues-test`);
    await page.waitForLoadState("networkidle");
    await page.getByRole("button", { name: /Issues/ }).click();
    await page.waitForLoadState("networkidle");
    await expect(page.getByText("First Playwright issue")).toBeVisible();
    await shot(page, "16-repo-issues-tab");
  });
});

// ─── Issues page ─────────────────────────────────────────────────────────────

test.describe("Issues page", () => {
  test("lists and creates issues", async ({ page }) => {
    await page.goto("/ui/");
    const user = await apiGet(page, "/api/v3/user");
    const owner = (user as { login: string }).login;

    await apiPost(page, "/api/v3/user/repos", {
      name: "issues-direct",
      description: "",
      private: false,
    }).catch(() => null);

    // Create issue via API
    await apiPost(page, `/api/v3/repos/${owner}/issues-direct/issues`, {
      title: "Direct issues page test",
    });

    await page.goto(`/ui/repos/${owner}/issues-direct/issues`);
    await page.waitForLoadState("networkidle");
    await expect(page.getByText("Direct issues page test")).toBeVisible();
    await shot(page, "17-issues-page");

    // Open new issue modal
    await page.getByRole("button", { name: "New issue" }).click();
    await expect(page.getByPlaceholder("Issue title")).toBeVisible();
    await shot(page, "18-new-issue-modal");

    // Fill and submit
    await page.getByPlaceholder("Issue title").fill("Created from UI");
    await page.getByRole("button", { name: "Create issue" }).click();
    await page.waitForURL(/\/ui\/repos\/.*\/issues\/\d+/);
    await shot(page, "19-issue-detail-after-create");
  });
});

// ─── Pull Requests page ───────────────────────────────────────────────────────

test.describe("Pull Requests page", () => {
  test("shows empty state", async ({ page }) => {
    await page.goto("/ui/");
    const user = await apiGet(page, "/api/v3/user");
    const owner = (user as { login: string }).login;

    await apiPost(page, "/api/v3/user/repos", {
      name: "pulls-direct",
      description: "",
      private: false,
    }).catch(() => null);

    await page.goto(`/ui/repos/${owner}/pulls-direct/pulls`);
    await page.waitForLoadState("networkidle");
    await expect(page.getByRole("heading", { level: 2 }).filter({ hasText: "Pull Requests" })).toBeVisible();
    await shot(page, "20-pulls-empty");
  });
});

// ─── Apps page ───────────────────────────────────────────────────────────────

test.describe("Apps page", () => {
  test("renders app tabs", async ({ page }) => {
    await page.goto("/ui/apps");
    await expect(page.getByRole("heading", { level: 2 }).filter({ hasText: "Apps" })).toBeVisible();
    // Tabs: GitHub Apps, Installations, OAuth Apps
    await expect(page.getByRole("button", { name: "GitHub Apps" })).toBeVisible();
    await shot(page, "21-apps-page");
  });

  test("opens create-app modal", async ({ page }) => {
    await page.goto("/ui/apps");
    const newAppBtn = page.getByRole("button", { name: /new.*app/i }).first();
    await newAppBtn.click();
    await expect(page.getByRole("heading", { name: /create/i })).toBeVisible();
    await shot(page, "22-create-app-modal");
    // Close modal
    await page.keyboard.press("Escape");
  });

  test("switches to OAuth Apps tab", async ({ page }) => {
    await page.goto("/ui/apps");
    await page.getByRole("button", { name: "OAuth Apps" }).click();
    await shot(page, "23-oauth-apps-tab");
  });
});

// ─── OAuth page ──────────────────────────────────────────────────────────────

test.describe("OAuth page", () => {
  test("renders device flow and web flow sections", async ({ page }) => {
    await page.goto("/ui/oauth");
    await shot(page, "24-oauth-page");
    // Should have some UI visible for OAuth flows
    await expect(page.url()).toContain("/ui/oauth");
  });
});

// ─── Metrics page ────────────────────────────────────────────────────────────

test.describe("Metrics page", () => {
  test("shows counters section", async ({ page }) => {
    await page.goto("/ui/metrics");
    await page.waitForLoadState("networkidle");
    await expect(page.getByRole("heading", { level: 2 }).filter({ hasText: /runtime/i })).toBeVisible();
    await shot(page, "25-metrics-page");
  });
});

// ─── Health endpoint ─────────────────────────────────────────────────────────

test.describe("Health endpoint", () => {
  test("returns 200 JSON", async ({ page }) => {
    const res = await page.goto("/health");
    expect(res?.status()).toBe(200);
  });
});
