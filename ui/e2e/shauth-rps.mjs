import assert from "node:assert/strict";
import { chromium } from "playwright";

const authOrigin = "http://localhost:8080";
const password = process.env.SHAUTH_BOOTSTRAP_ADMIN_PASSWORD;
assert.ok(password, "SHAUTH_BOOTSTRAP_ADMIN_PASSWORD is required");

const apps = [
  { name: "Sockerless Admin", launch: "http://localhost:29090/ui/", identity: "http://localhost:29090/auth/session", signedOut: "http://localhost:29090/auth/signed-out" },
  { name: "Sockerless AWS simulator", launch: "http://localhost:29310/ui/", identity: "http://localhost:29310/auth/session", signedOut: "http://localhost:29310/auth/signed-out" },
  { name: "Sockerless Google Cloud simulator", launch: "http://localhost:29320/ui/", identity: "http://localhost:29320/auth/session", signedOut: "http://localhost:29320/auth/signed-out" },
  { name: "Sockerless Microsoft Azure simulator", launch: "http://localhost:29330/ui/", identity: "http://localhost:29330/auth/session", signedOut: "http://localhost:29330/auth/signed-out" },
];

const browser = await chromium.launch({ headless: true });
try {
  for (const app of apps) {
    const context = await browser.newContext();
    const page = await context.newPage();
    const failures = monitor(page);
    await page.goto(app.launch, { waitUntil: "domcontentloaded" });
    await signInIfRequired(page);
    await waitForOrigin(page, new URL(app.launch).origin);
    await assertIdentity(context, app);
    assert.deepEqual(failures, [], `${app.name} direct login emitted browser failures`);
    await context.close();
  }

  const portalContext = await browser.newContext();
  const portalPage = await portalContext.newPage();
  const portalFailures = monitor(portalPage);
  await portalPage.goto(`${authOrigin}/apps`, { waitUntil: "domcontentloaded" });
  await signInIfRequired(portalPage);
  for (const app of apps) {
    await portalPage.goto(`${authOrigin}/apps`, { waitUntil: "domcontentloaded" });
    const appLink = portalPage.getByRole("link", { name: `Open ${app.name}`, exact: true });
    await appLink.waitFor({ state: "visible" });
    await appLink.click();
    await waitForOrigin(portalPage, new URL(app.launch).origin);
    assert.notEqual(new URL(portalPage.url()).origin, authOrigin, `${app.name} catalog launch remained on Shauth`);
    await assertIdentity(portalContext, app);
  }
  assert.deepEqual(portalFailures, [], "catalog SSO emitted browser failures");
  await portalContext.close();

  for (let index = 0; index < apps.length; index += 1) {
    const app = apps[index];
    const sentinel = apps[(index + 1) % apps.length];
    const context = await browser.newContext();
    const page = await context.newPage();
    const failures = monitor(page);

    await page.goto(app.launch, { waitUntil: "domcontentloaded" });
    await signInIfRequired(page);
    await waitForOrigin(page, new URL(app.launch).origin);
    await assertIdentity(context, app);

    await page.goto(sentinel.launch, { waitUntil: "domcontentloaded" });
    assert.notEqual(new URL(page.url()).pathname, "/login", `${sentinel.name} prompted after shared SSO was established`);
    await waitForOrigin(page, new URL(sentinel.launch).origin);
    await assertIdentity(context, sentinel);

    await page.goto(app.launch, { waitUntil: "domcontentloaded" });
    await page.locator('form[action="/auth/logout"] button').click();
    await page.waitForURL(app.signedOut, { timeout: 30_000 });
    assert.equal(page.url(), app.signedOut, `${app.name} logout did not finish on the originating app`);
    assert.equal((await context.request.get(app.identity)).status(), 401, `${app.name} local session survived logout`);
    await page.reload({ waitUntil: "domcontentloaded" });
    assert.equal(page.url(), app.signedOut, `${app.name} signed-out page restarted authentication after reload`);

    await page.goto(`${authOrigin}/apps`, { waitUntil: "domcontentloaded" });
    await waitForShauthLogin(page);
    await page.goto(sentinel.launch, { waitUntil: "domcontentloaded" });
    await waitForShauthLogin(page);
    assert.deepEqual(failures, [], `${app.name} global logout emitted browser failures`);
    await context.close();
  }
} finally {
  await browser.close();
}

async function signInIfRequired(page) {
  if (new URL(page.url()).origin !== authOrigin) return;
  await page.locator("#username").fill("admin");
  await page.locator("#password").fill(password);
  await page.getByRole("button", { name: "Sign in with password" }).click();
}

async function waitForOrigin(page, origin) {
  await page.waitForURL((raw) => new URL(raw.toString()).origin === origin && !new URL(raw.toString()).pathname.includes("callback"), { timeout: 30_000 });
}

async function waitForShauthLogin(page) {
  await page.waitForURL((raw) => {
    const target = new URL(raw.toString());
    return target.origin === authOrigin && target.pathname === "/login";
  }, { timeout: 30_000 });
}

async function assertIdentity(context, app) {
  const response = await context.request.get(app.identity);
  assert.equal(response.status(), 200, `${app.name} identity endpoint returned ${response.status()}`);
  const identity = await response.json();
  assert.equal(identity.authenticated, true, `${app.name} did not expose an authenticated identity`);
}

function monitor(page) {
  const failures = [];
  page.on("pageerror", (error) => failures.push(`page error: ${error.message}`));
  page.on("requestfailed", (request) => {
    const reason = request.failure()?.errorText ?? "unknown error";
    if (request.isNavigationRequest() && reason === "net::ERR_ABORTED") return;
    failures.push(`request failed (${reason}): ${new URL(request.url()).origin}${new URL(request.url()).pathname}`);
  });
  page.on("response", (response) => {
    if (response.request().isNavigationRequest() && response.status() >= 400) {
      failures.push(`document ${response.status()}: ${new URL(response.url()).origin}${new URL(response.url()).pathname}`);
    }
  });
  return failures;
}
