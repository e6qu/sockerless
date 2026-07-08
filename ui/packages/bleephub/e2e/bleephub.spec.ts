import { test, expect, type Page } from "@playwright/test";
import fs from "fs";
import path from "path";

// Fail e2e tests on browser console errors or uncaught page exceptions.
// Warnings are logged to stdout for audit visibility but do not fail the test.
test.beforeEach(({ page }, testInfo) => {
  page.on("console", (msg) => {
    const text = msg.text();
    const type = msg.type();
    if (type === "error") {
      throw new Error(`Console error in ${testInfo.title}: ${text}`);
    }
    if (type === "warning") {
      // eslint-disable-next-line no-console
      console.warn(`[browser warning] ${testInfo.title}: ${text}`);
    }
  });
  page.on("pageerror", (err) => {
    throw new Error(`Uncaught page error in ${testInfo.title}: ${err.message}`);
  });
});

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
      if (res.status === 204) return null;
      const text = await res.text();
      return text ? JSON.parse(text) : null;
    },
    { base: BASE, path, token: TOKEN, body },
  );
}

async function apiPut(page: Page, path: string, body: unknown) {
  return page.evaluate(
    async ({ base, path, token, body }) => {
      const res = await fetch(base + path, {
        method: "PUT",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
        body: JSON.stringify(body),
      });
      if (!res.ok) throw new Error(`${res.status} ${res.statusText} ${await res.text()}`);
      const text = await res.text();
      return text ? JSON.parse(text) : null;
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

// Open the GitHub-style global-nav drawer (the hamburger). The drawer holds
// both the GitHub destinations and the bleephub "Operations" section.
async function openDrawer(page: Page): Promise<void> {
  await page.getByRole("button", { name: "Open global navigation" }).click();
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

// ─── Operations console (/ui/admin) ──────────────────────────────────────────
// The server-operational "System status" console lives at /ui/admin; the root
// /ui/ is the GitHub-style dashboard.

test.describe("Operations console", () => {
  test("renders the System status heading", async ({ page }) => {
    await page.goto("/ui/admin");
    // Brand is a link in the header; the page title is the h1.
    await expect(page.getByRole("link", { name: "bleephub" })).toBeVisible();
    await expect(page.getByRole("heading", { level: 1 }).filter({ hasText: "System status" })).toBeVisible();
    await shot(page, "01-ops-console");
  });

  test("shows metrics cards", async ({ page }) => {
    await page.goto("/ui/admin");
    await expect(page.getByText("Active Workflows")).toBeVisible();
    await expect(page.getByText("Connected Runners")).toBeVisible();
    await expect(page.getByText("Submissions")).toBeVisible();
    await shot(page, "02-ops-metrics");
  });
});

// ─── Global-nav drawer ───────────────────────────────────────────────────────

test.describe("Global navigation", () => {
  test("the drawer lists the GitHub and Operations destinations", async ({ page }) => {
    await page.goto("/ui/");
    await openDrawer(page);
    const drawer = page.getByRole("navigation", { name: "Global" });
    await expect(drawer.getByRole("link", { name: "Repositories" })).toBeVisible();
    await expect(drawer.getByRole("link", { name: "Runners" })).toBeVisible();
    await expect(drawer.getByRole("link", { name: "GitHub Apps" })).toBeVisible();
    await expect(drawer.getByRole("link", { name: "OAuth Apps" })).toBeVisible();
    await expect(drawer.getByRole("link", { name: "Metrics" })).toBeVisible();
    await expect(drawer.getByRole("link", { name: "System status" })).toBeVisible();
    await shot(page, "03-nav-drawer");
  });

  test("navigates between pages via the drawer", async ({ page }) => {
    await page.goto("/ui/");
    const drawer = page.getByRole("navigation", { name: "Global" });

    await openDrawer(page);
    await drawer.getByRole("link", { name: "Runners" }).click();
    await expect(page).toHaveURL(/\/ui\/runners/);
    await expect(page.getByRole("heading", { level: 1 }).filter({ hasText: "Runners" })).toBeVisible();
    await shot(page, "04-runners");

    await openDrawer(page);
    await drawer.getByRole("link", { name: "Repositories" }).click();
    await expect(page).toHaveURL(/\/ui\/repos/);
    await expect(page.getByRole("heading", { level: 1 }).filter({ hasText: "Repositories" })).toBeVisible();
    await shot(page, "05-repos");

    await openDrawer(page);
    await drawer.getByRole("link", { name: "GitHub Apps" }).click();
    await expect(page).toHaveURL(/\/ui\/apps/);
    await expect(page.getByRole("heading", { level: 1 }).filter({ hasText: "Apps" })).toBeVisible();
    await shot(page, "06-apps");

    await openDrawer(page);
    await drawer.getByRole("link", { name: "OAuth Apps" }).click();
    await expect(page).toHaveURL(/\/ui\/oauth/);
    await shot(page, "07-oauth");

    await openDrawer(page);
    await drawer.getByRole("link", { name: "Metrics" }).click();
    await expect(page).toHaveURL(/\/ui\/metrics/);
    await expect(page.getByRole("heading", { level: 1 }).filter({ hasText: /runtime/i })).toBeVisible();
    await shot(page, "08-metrics");

    await openDrawer(page);
    await drawer.getByRole("link", { name: "Dashboard" }).click();
    await expect(page).toHaveURL(/\/ui\/$/);
    await shot(page, "09-back-dashboard");
  });
});

// ─── Dark / light mode toggle ────────────────────────────────────────────────

test.describe("Theme toggle", () => {
  test("theme toggle lives in the user menu", async ({ page }) => {
    await page.goto("/ui/");
    await page.waitForLoadState("networkidle");
    // The theme toggle is an item in the avatar dropdown menu.
    await page.getByRole("button", { name: "Open user menu" }).click();
    const toggle = page.getByRole("menuitem", { name: /(light|dark) theme/i });
    await expect(toggle).toBeVisible();
    await shot(page, "11-theme-toggle");
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

  test("creates a repo through the UI dialog", async ({ page }) => {
    await page.goto("/ui/repos");
    await page.waitForLoadState("networkidle");

    await page.getByRole("button", { name: "New repository" }).click();
    await expect(page.getByRole("heading", { name: "Create a new repository" })).toBeVisible();

    await page.getByLabel("Repository name").fill("ui-created-repo");
    await page.getByLabel("Description").fill("Created from the UI");
    await page.getByRole("button", { name: "Create repository" }).click();

    await expect(page.getByRole("link", { name: /ui-created-repo/ })).toBeVisible();
    await shot(page, "13b-repos-after-ui-create");
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

    // Repo header renders owner / repo as separate links; the empty Code
    // view shows the clone blankslate.
    await expect(page.getByRole("link", { name: "detail-test" })).toBeVisible();
    await expect(page.getByText(/this repository is empty/i)).toBeVisible();
    await expect(page.getByRole("button", { name: "HTTPS" })).toBeVisible();
    await expect(page.getByRole("button", { name: "SSH" })).toBeVisible();
    await expect(page.getByRole("button", { name: "GitHub CLI" })).toBeVisible();
    await shot(page, "15-repo-detail-empty");
  });

  test("shows file tree and rendered README for initialized repo", async ({ page }) => {
    await page.goto("/ui/");
    const user = await apiGet(page, "/api/v3/user");
    const owner = (user as { login: string }).login;

    await apiPost(page, "/api/v3/user/repos", {
      name: "readme-test",
      description: "README test",
      private: false,
      auto_init: true,
    }).catch(() => null);

    await page.goto(`/ui/repos/${owner}/readme-test`);
    await page.waitForLoadState("networkidle");

    await expect(page.getByText("README.md").first()).toBeVisible();
    await expect(page.getByRole("combobox", { name: "Branch" })).toHaveValue("main");
    await shot(page, "15b-repo-detail-with-readme");
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
    // Issues is a repo tab (link) in the repo header. Scope to the repo nav so
    // it doesn't collide with the global header's "Issues" quick-link.
    await page.getByRole("navigation", { name: "Repository" }).getByRole("link", { name: /Issues/ }).click();
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
    await expect(page.getByText(/no open pull requests/i)).toBeVisible();
    await shot(page, "20-pulls-empty");
  });
});

// ─── Apps page ───────────────────────────────────────────────────────────────

test.describe("Apps page", () => {
  test("renders app tabs", async ({ page }) => {
    await page.goto("/ui/apps");
    await expect(page.getByRole("heading", { level: 1 }).filter({ hasText: "Apps" })).toBeVisible();
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

// ─── Actions (workflows + runs + detail) ─────────────────────────────────────

test.describe("Actions UI", () => {
  test("submits a workflow and renders the run detail (jobs + logs section)", async ({ page }) => {
    await page.goto("/ui/");
    const repoName = `ci-demo-${Date.now()}`;
    const repoFullName = `admin/${repoName}`;
    await apiPost(page, "/api/v3/user/repos", { name: repoName, auto_init: true });

    // Commit a real GitHub Actions workflow file and dispatch it through the
    // public workflow-dispatch application programming interface. No runner is
    // attached in this test, so logs stay empty; the detail page must still
    // render the job table and logs view.
    const yaml = [
      "name: CI Pipeline",
      "on: workflow_dispatch",
      "jobs:",
      "  build:",
      "    runs-on: ubuntu-latest",
      "    steps:",
      "      - run: echo building",
      "  test:",
      "    runs-on: ubuntu-latest",
      "    needs: build",
      "    steps:",
      "      - run: echo testing",
    ].join("\n");
    await apiPut(page, `/api/v3/repos/${repoFullName}/contents/.github/workflows/ci.yml`, {
      message: "Add GitHub Actions workflow",
      content: Buffer.from(yaml, "utf8").toString("base64"),
      branch: "main",
    });
    await apiPost(page, `/api/v3/repos/${repoFullName}/actions/workflows/ci.yml/dispatches`, { ref: "main", inputs: {} });

    // Runs tab lists the run (the tab is a button; the page title also
    // contains the word "runs", so target the button role explicitly).
    await page.goto("/ui/workflows");
    await page.waitForLoadState("networkidle");
    await page.getByRole("button", { name: "Runs" }).click();
    await expect(page.getByText("CI Pipeline").first()).toBeVisible();
    await shot(page, "29-actions-runs");

    // Click the run row → detail page with the job table + logs section.
    await page.getByText("CI Pipeline").first().click();
    await page.waitForURL(/\/ui\/workflows\/.+/);
    await page.waitForLoadState("networkidle");
    await expect(page.getByRole("heading", { name: /Jobs/i })).toBeVisible();
    await shot(page, "30-actions-run-detail");
  });
});

// ─── Metrics page ────────────────────────────────────────────────────────────

test.describe("Metrics page", () => {
  test("shows counters section", async ({ page }) => {
    await page.goto("/ui/metrics");
    await page.waitForLoadState("networkidle");
    await expect(page.getByRole("heading", { level: 1 }).filter({ hasText: /runtime/i })).toBeVisible();
    await shot(page, "25-metrics-page");
  });
});

// ─── Dark theme capture ──────────────────────────────────────────────────────

test.describe("Dark theme", () => {
  // Seed the persisted theme to "dark" before any script runs so the app
  // boots in dark mode, then capture the key surfaces. Runs after the repo
  // /issue tests above, so their data is still in the in-memory store.
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      window.localStorage.setItem("sockerless:theme", "dark");
    });
  });

  test("captures key pages in dark mode", async ({ page }) => {
    await page.goto("/ui/admin");
    await page.waitForLoadState("networkidle");
    await expect(page.getByRole("heading", { level: 1 }).filter({ hasText: "System status" })).toBeVisible();
    await shot(page, "26-dark-ops-console");

    await page.goto("/ui/repos");
    await page.waitForLoadState("networkidle");
    await shot(page, "27-dark-repos");

    const user = await apiGet(page, "/api/v3/user");
    const owner = (user as { login: string }).login;
    await page.goto(`/ui/repos/${owner}/issues-direct/issues`);
    await page.waitForLoadState("networkidle");
    await shot(page, "28-dark-issues");
  });
});

// ─── Health endpoint ─────────────────────────────────────────────────────────

test.describe("Health endpoint", () => {
  test("returns 200 JSON", async ({ page }) => {
    const res = await page.goto("/health");
    expect(res?.status()).toBe(200);
  });
});
