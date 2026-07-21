import assert from "node:assert/strict";
import { chromium } from "playwright";

const authOrigin = "http://localhost:8080";
const password = process.env.SHAUTH_BOOTSTRAP_ADMIN_PASSWORD;
const developerPassword = process.env.SHAUTH_DEVELOPER_PASSWORD;
assert.ok(password, "SHAUTH_BOOTSTRAP_ADMIN_PASSWORD is required");
assert.ok(developerPassword, "SHAUTH_DEVELOPER_PASSWORD is required");

const apps = [
  { name: "Sockerless Admin", launch: "http://localhost:29090/ui/", identity: "http://localhost:29090/auth/session", bridge: "http://localhost:29090/auth/shauth/logout/complete", signedOut: "http://localhost:29090/auth/signed-out", login: "/auth/shauth" },
  { name: "Sockerless AWS simulator", launch: "http://localhost:29310/ui/", identity: "http://localhost:29310/auth/session", bridge: "http://localhost:29310/auth/shauth/logout/complete", signedOut: "http://localhost:29310/auth/signed-out", login: "/auth/oidc/login" },
  { name: "Sockerless Google Cloud simulator", launch: "http://localhost:29320/ui/", identity: "http://localhost:29320/auth/session", bridge: "http://localhost:29320/auth/shauth/logout/complete", signedOut: "http://localhost:29320/auth/signed-out", login: "/auth/oidc/login" },
  { name: "Sockerless Microsoft Azure simulator", launch: "http://localhost:29330/ui/", identity: "http://localhost:29330/auth/session", bridge: "http://localhost:29330/auth/shauth/logout/complete", signedOut: "http://localhost:29330/auth/signed-out", login: "/auth/oidc/login" },
];

const browser = await chromium.launch({ headless: true });
try {
  await provisionDeveloper();
  await assertDeveloperCannotAccessAdmin();

  for (const app of apps) {
    const context = await browser.newContext();
    const page = await context.newPage();
    const failures = monitor(page);
    await page.goto(app.launch, { waitUntil: "domcontentloaded" });
    await signInIfRequired(page);
    await waitForApplication(page, app);
    await assertIdentity(context, app);
    if (app.name === "Sockerless Admin") await assertAdministratorMutation(context);
    await logoutFromApplication(page, context, app);
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
    await waitForApplication(portalPage, app);
    assert.notEqual(new URL(portalPage.url()).origin, authOrigin, `${app.name} catalog launch remained on Shauth`);
    await assertIdentity(portalContext, app);
  }
  await logoutFromShauth(portalPage);
  for (const app of apps) {
    assert.equal((await portalContext.request.get(app.identity, { maxRedirects: 0 })).status(), 401, `${app.name} session survived provider logout`);
    await portalPage.goto(app.launch, { waitUntil: "domcontentloaded" });
    await waitForShauthLogin(portalPage);
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
    await waitForApplication(page, app);
    await assertIdentity(context, app);

    await page.goto(sentinel.launch, { waitUntil: "domcontentloaded" });
    assert.notEqual(new URL(page.url()).pathname, "/login", `${sentinel.name} prompted after shared SSO was established`);
    await waitForApplication(page, sentinel);
    await assertIdentity(context, sentinel);

    await page.goto(app.launch, { waitUntil: "domcontentloaded" });
    await waitForApplication(page, app);
    await logoutFromApplication(page, context, app);

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

async function logoutFromApplication(page, context, app) {
  const bridgeRequest = page.waitForRequest((request) => request.resourceType() === "document" && request.url() === app.bridge);
  await page.locator('form[action="/auth/logout"] button').click();
  assert.equal((await bridgeRequest).url(), app.bridge, `${app.name} did not traverse its exact logout-completion bridge`);
  await page.waitForURL(app.signedOut, { timeout: 30_000 });
  assert.equal(page.url(), app.signedOut, `${app.name} logout did not finish on the originating app`);
  assert.equal((await context.request.get(app.identity, { maxRedirects: 0 })).status(), 401, `${app.name} local session survived logout`);
  await page.reload({ waitUntil: "domcontentloaded" });
  assert.equal(page.url(), app.signedOut, `${app.name} signed-out page restarted authentication after reload`);

  const signIn = page.getByRole("link", { name: "Sign in with Shauth", exact: true });
  await signIn.waitFor({ state: "visible" });
  assert.equal(await signIn.getAttribute("href"), app.login, `${app.name} exposed the wrong Shauth sign-in start`);
  await signIn.click();
  await waitForShauthLogin(page);
}

async function logoutFromShauth(page) {
  await page.goto(`${authOrigin}/logout`, { waitUntil: "domcontentloaded" });
  await page.getByRole("button", { name: "Sign out of all apps", exact: true }).click();
  await page.waitForURL(`${authOrigin}/signed-out`, { timeout: 30_000 });
}

async function signInIfRequired(page, username = "admin", accountPassword = password) {
  if (new URL(page.url()).origin !== authOrigin) return;
  await page.locator("#username").fill(username);
  await page.locator("#password").fill(accountPassword);
  await page.getByRole("button", { name: "Sign in with password" }).click();
}

async function provisionDeveloper() {
  const context = await browser.newContext();
  const page = await context.newPage();
  try {
    await page.goto(`${authOrigin}/admin/users`, { waitUntil: "domcontentloaded" });
    await signInIfRequired(page);
    await page.goto(`${authOrigin}/admin/users`, { waitUntil: "domcontentloaded" });
    await page.locator("#new-username").fill("sockerless-developer");
    await page.locator("#new-email").fill("sockerless-developer@localhost.test");
    await page.locator("#new-password").fill(developerPassword);
    await page.locator("#new-role").selectOption("developer");
    await page.getByRole("button", { name: "Create local user" }).click();
    await page.getByRole("link", { name: "sockerless-developer", exact: true }).waitFor({ state: "visible" });
  } finally {
    await context.close();
  }
}

async function assertDeveloperCannotAccessAdmin() {
  const context = await browser.newContext();
  const page = await context.newPage();
  const failures = monitor(page, new Set([403]));
  try {
    await page.goto(apps[0].launch, { waitUntil: "domcontentloaded" });
    await signInIfRequired(page, "sockerless-developer", developerPassword);
    await page.waitForURL(apps[0].launch, { timeout: 30_000 });
    await page.getByRole("heading", { name: "Administrator access required" }).waitFor({ state: "visible" });

    const identityResponse = await context.request.get(apps[0].identity);
    assert.equal(identityResponse.status(), 200, "developer identity endpoint was unavailable");
    const identity = await identityResponse.json();
    assert.equal(identity.authenticated, true, "developer session was not authenticated");
    assert.equal(identity.role, "developer", "developer session did not preserve its Shauth role");

    const mutation = await context.request.post("http://localhost:29090/api/v1/topology/projects", {
      data: { name: "developer-must-not-create", instances: [] },
    });
    assert.equal(mutation.status(), 403, "developer could mutate Sockerless Admin topology");
    assert.match((await mutation.json()).error, /administrator role required/i);
    assert.deepEqual(failures, [], "developer authorization denial emitted browser failures");
  } finally {
    await context.close();
  }
}

async function assertAdministratorMutation(context) {
  const name = "authorization-matrix-proof";
  const create = await context.request.post("http://localhost:29090/api/v1/topology/projects", {
    data: { name, instances: [] },
  });
  assert.equal(create.status(), 201, `administrator topology create returned ${create.status()}`);

  const topologyResponse = await context.request.get("http://localhost:29090/api/v1/topology");
  assert.equal(topologyResponse.status(), 200, `administrator topology read returned ${topologyResponse.status()}`);
  const topology = await topologyResponse.json();
  assert.ok(topology.projects.some((project) => project.name === name), "administrator mutation was not persisted");

  const remove = await context.request.delete(`http://localhost:29090/api/v1/topology/projects/${name}`);
  assert.equal(remove.status(), 200, `administrator topology cleanup returned ${remove.status()}`);
}

async function waitForOrigin(page, origin) {
  await page.waitForURL((raw) => new URL(raw.toString()).origin === origin && !new URL(raw.toString()).pathname.includes("callback"), { timeout: 30_000 });
}

async function waitForApplication(page, app) {
  await waitForOrigin(page, new URL(app.launch).origin);
  await page.waitForLoadState("networkidle");
  await page.locator('form[action="/auth/logout"] button').waitFor({ state: "visible" });
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

function monitor(page, allowedDocumentStatuses = new Set()) {
  const failures = [];
  page.on("pageerror", (error) => failures.push(`page error: ${error.message}`));
  page.on("requestfailed", (request) => {
    const reason = request.failure()?.errorText ?? "unknown error";
    if (request.isNavigationRequest() && reason === "net::ERR_ABORTED") return;
    failures.push(`request failed (${reason}): ${new URL(request.url()).origin}${new URL(request.url()).pathname}`);
  });
  page.on("response", (response) => {
    if (response.request().isNavigationRequest() && response.status() >= 400 && !allowedDocumentStatuses.has(response.status())) {
      failures.push(`document ${response.status()}: ${new URL(response.url()).origin}${new URL(response.url()).pathname}`);
    }
  });
  return failures;
}
