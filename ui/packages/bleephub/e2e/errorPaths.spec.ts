import { test, expect } from "@playwright/test";

// Graceful-degradation e2e: drive the real bleephub app against real error
// responses (a 404 for a repo that does not exist) and against a fault
// injected at the network layer (a fulfilled HTTP 500), and assert the app
// degrades to a visible error surface — never a blank screen, never an
// uncaught page exception that blanks the SPA. A fulfilled 500 (not an
// aborted request) keeps the browser console clean, so the only failure
// signal is a genuine app crash, which `pageerror` below turns into a
// failing test.
test.beforeEach(({ page }, testInfo) => {
  page.on("pageerror", (err) => {
    throw new Error(`Uncaught page error in ${testInfo.title}: ${err.message}`);
  });
});

test.describe("Error handling / fault injection", () => {
  test("a nonexistent repo's Insights renders a visible error, not a blank page", async ({
    page,
  }) => {
    await page.goto("/ui/repos/admin/this-repo-does-not-exist-xyz/insights");
    await page.waitForLoadState("networkidle");

    // The repo breadcrumb (param-driven, no fetch) still renders — the shell
    // survives the failed data fetches.
    await expect(
      page.getByRole("link", { name: "this-repo-does-not-exist-xyz" }),
    ).toBeVisible();
    // Each Insights panel 404s and degrades to its own InlineError.
    await expect(page.getByText(/Failed to load/i).first()).toBeVisible();
  });

  test("an injected 500 on the repos list degrades to a visible error", async ({ page }) => {
    // Fulfil (do not abort) so the browser sees a normal HTTP 500 — the app's
    // own error handling must take it from there.
    await page.route("**/api/v3/user/repos**", (route) =>
      route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ message: "Internal Server Error" }),
      }),
    );

    await page.goto("/ui/repos");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText(/Failed to load repositories/i)).toBeVisible();
    // The app shell (the header brand) is still present — the whole app did not
    // fall over.
    await expect(page.getByRole("link", { name: "bleephub" })).toBeVisible();
  });

  test("an injected 500 on the public repository list degrades the Operations console to a visible error", async ({ page }) => {
    await page.route("**/api/v3/user/repos**", (route) =>
      route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ message: "Internal Server Error" }),
      }),
    );

    // The GitHub Actions metrics-driven "System status" console lives at /ui/admin.
    await page.goto("/ui/admin");
    await page.waitForLoadState("networkidle");

    // A failed public aggregate fetch must degrade to a visible InlineError,
    // never a blank console or an uncaught render. The app shell survives.
    await expect(page.getByText(/Failed to load overview/i)).toBeVisible();
    await expect(page.getByRole("link", { name: "bleephub" })).toBeVisible();
  });
});
