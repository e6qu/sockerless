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
    await assertIdentity(page, context, app);
    if (app.name === "Sockerless Admin") await assertAdministratorMutation(context);
    if (app.name === "Sockerless Google Cloud simulator") {
      await assertFederatedCloudToken(context, page, app);
      await assertMintedServiceAccountKey(page, app);
    }
    if (app.name === "Sockerless AWS simulator") {
      await assertFederatedAwsCredentials(context, page, app);
      await assertMintedIAMAccessKey(page, app);
    }
    if (app.name === "Sockerless Microsoft Azure simulator") {
      await assertFederatedAzureToken(context, app);
      await assertMintedEntraClientSecret(page, app);
    }
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
    await assertIdentity(portalPage, portalContext, app);
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
    await assertIdentity(page, context, app);

    await page.goto(sentinel.launch, { waitUntil: "domcontentloaded" });
    assert.notEqual(new URL(page.url()).pathname, "/login", `${sentinel.name} prompted after shared SSO was established`);
    await waitForApplication(page, sentinel);
    await assertIdentity(page, context, sentinel);

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
  // Some consoles nest sign-out inside an account menu behind the avatar, the
  // way the real cloud console does; open it first when present.
  await openAccountMenu(page);
  await page.locator("[data-shauth-sign-out]").click();
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

// assertFederatedCloudToken proves the console federates the signed-in
// operator's real Shauth assertion into a cloud access token the way it reaches
// the real cloud: it reads the assertion from the console's own auth layer, then
// exchanges it at the cloud's real Security Token Service — no simulator-served
// broker. The assertion is verified against Shauth's own discovery and key set,
// so a token here means the whole federation works end to end with a live
// identity, differing from the real cloud only in coordinates.
// assertFederatedAwsCredentials proves the AWS console federates the signed-in
// operator's real Shauth assertion into temporary credentials the way it reaches
// AWS: it reads the assertion from the console's own auth layer, exchanges it at
// the Security Token Service through AssumeRoleWithWebIdentity, and drives the
// signed-in console through a real API read over the resulting credentials.
async function assertFederatedAwsCredentials(context, page, app) {
  const origin = new URL(app.launch).origin;

  const subjectResponse = await context.request.get(`${origin}/auth/federation-subject`);
  assert.equal(subjectResponse.status(), 200, `${app.name} auth layer did not expose the operator assertion`);
  const { subject_token: subjectToken } = await subjectResponse.json();
  assert.ok(subjectToken, `${app.name} exposed no operator assertion to federate`);

  const exchange = await context.request.post(`${origin}/`, {
    form: {
      Action: "AssumeRoleWithWebIdentity",
      Version: "2011-06-15",
      RoleArn: "arn:aws:iam::123456789012:role/console-federation-role",
      RoleSessionName: "console",
      WebIdentityToken: subjectToken,
    },
  });
  assert.equal(exchange.status(), 200, `${app.name} AssumeRoleWithWebIdentity returned ${exchange.status()}`);
  const credentials = await exchange.text();
  assert.match(credentials, /<AccessKeyId>ASIA/, `${app.name} exchange returned no temporary credentials`);

  await page.goto(`${origin}/ui/ecs`, { waitUntil: "domcontentloaded" });
  await page.getByRole("heading", { name: "Tasks" }).waitFor({ state: "visible" });
  assert.equal(
    await page.getByText("Could not load").count(),
    0,
    `${app.name} console failed to read the real API over federation`,
  );
}

async function assertFederatedCloudToken(context, page, app) {
  const origin = new URL(app.launch).origin;

  const subjectResponse = await context.request.get(`${origin}/auth/federation-subject`);
  assert.equal(subjectResponse.status(), 200, `${app.name} auth layer did not expose the operator assertion`);
  const { subject_token: subjectToken } = await subjectResponse.json();
  assert.ok(subjectToken, `${app.name} exposed no operator assertion to federate`);

  const exchange = await context.request.post(`${origin}/v1/token`, {
    form: {
      grant_type: "urn:ietf:params:oauth:grant-type:token-exchange",
      audience: "//iam.googleapis.com/locations/global/workforcePools/sockerless-console/providers/sso",
      subject_token: subjectToken,
      subject_token_type: "urn:ietf:params:oauth:token-type:jwt",
      requested_token_type: "urn:ietf:params:oauth:token-type:access_token",
    },
  });
  assert.equal(exchange.status(), 200, `${app.name} Security Token Service exchange returned ${exchange.status()}`);
  const token = await exchange.json();
  assert.ok(token.access_token, `${app.name} exchange issued no federated token`);
  assert.equal(token.token_type, "Bearer", `${app.name} federated token was not a bearer token`);

  // The signed-in console reads a real cloud API through that federation; a load
  // error would mean the SPA's federation path is broken.
  await page.goto(`${origin}/ui/cloudrun`, { waitUntil: "domcontentloaded" });
  await page.getByRole("heading", { name: "Cloud Run jobs" }).waitFor({ state: "visible" });
  assert.equal(
    await page.getByText("Couldn't load").count(),
    0,
    `${app.name} console failed to read the real Cloud Run API over federation`,
  );
}

// assertFederatedAzureToken proves the Azure portal federates the signed-in
// operator's real Shauth assertion into an Azure Resource Manager token the way
// it reaches Azure: an administrator registers a user-assigned managed identity
// and a federated identity credential trusting the operator's issuer, subject,
// and audience, and Microsoft Entra exchanges the assertion for a token. The
// exchange is the security-critical, novel path; the portal's real Azure
// Resource Manager reads over that token are covered by the Playwright suite,
// which seeds resources through the same APIs and asserts they render.
async function assertFederatedAzureToken(context, app) {
  const origin = new URL(app.launch).origin;
  const sub = "00000000-0000-0000-0000-000000000001";
  const rg = "console-federation-rg";
  const identityName = "console-identity";

  const subjectResponse = await context.request.get(`${origin}/auth/federation-subject`);
  assert.equal(subjectResponse.status(), 200, `${app.name} auth layer did not expose the operator assertion`);
  const { subject_token: subjectToken } = await subjectResponse.json();
  assert.ok(subjectToken, `${app.name} exposed no operator assertion to federate`);

  // The federated identity credential pins the operator's own issuer, subject,
  // and audience, so read them from the assertion the way an administrator reads
  // them off a real token before registering the trust.
  const claims = JSON.parse(Buffer.from(subjectToken.split(".")[1], "base64url").toString("utf8"));
  const audience = Array.isArray(claims.aud) ? claims.aud[0] : claims.aud;

  // The Azure simulator enforces bearer authentication on its Azure Resource
  // Manager plane exactly as real ARM does: a request without a token whose
  // `aud` is the management audience is rejected with 401. The administrator
  // provisioning below therefore acquires an ARM token through a client
  // credentials grant — the same flow a real admin client (az login as a service
  // principal, azidentity) uses — and presents it on every ARM write. The token
  // endpoint is exempt because it is how a client obtains the credential.
  const adminTokenResponse = await context.request.post(`${origin}/organizations/oauth2/v2.0/token`, {
    form: {
      grant_type: "client_credentials",
      client_id: "console-provisioning-admin",
      scope: "https://management.azure.com/.default",
    },
  });
  assert.equal(adminTokenResponse.status(), 200, `${app.name} admin ARM token request returned ${adminTokenResponse.status()}`);
  const { access_token: adminToken } = await adminTokenResponse.json();
  assert.ok(adminToken, `${app.name} issued no admin ARM token to provision with`);
  const armAuth = { Authorization: `Bearer ${adminToken}` };

  const rgResponse = await context.request.put(
    `${origin}/subscriptions/${sub}/resourcegroups/${rg}?api-version=2021-04-01`,
    { headers: armAuth, data: { location: "eastus" } },
  );
  assert.ok(rgResponse.ok(), `${app.name} resource group create returned ${rgResponse.status()}`);

  const identityResponse = await context.request.put(
    `${origin}/subscriptions/${sub}/resourceGroups/${rg}/providers/Microsoft.ManagedIdentity/userAssignedIdentities/${identityName}?api-version=2024-11-30`,
    { headers: armAuth, data: { location: "eastus" } },
  );
  assert.ok(identityResponse.ok(), `${app.name} managed identity create returned ${identityResponse.status()}`);
  const { properties: identity } = await identityResponse.json();
  assert.ok(identity?.clientId, `${app.name} managed identity has no client id`);

  const ficResponse = await context.request.put(
    `${origin}/subscriptions/${sub}/resourceGroups/${rg}/providers/Microsoft.ManagedIdentity/userAssignedIdentities/${identityName}/federatedIdentityCredentials/console-fic?api-version=2024-11-30`,
    { headers: armAuth, data: { properties: { issuer: claims.iss, subject: claims.sub, audiences: [audience] } } },
  );
  assert.ok(ficResponse.ok(), `${app.name} federated identity credential create returned ${ficResponse.status()}`);

  const exchange = await context.request.post(`${origin}/organizations/oauth2/v2.0/token`, {
    form: {
      grant_type: "client_credentials",
      client_id: identity.clientId,
      scope: "https://management.azure.com/.default",
      client_assertion_type: "urn:ietf:params:oauth:client-assertion-type:jwt-bearer",
      client_assertion: subjectToken,
    },
  });
  assert.equal(exchange.status(), 200, `${app.name} Microsoft Entra token exchange returned ${exchange.status()}`);
  const token = await exchange.json();
  assert.ok(token.access_token, `${app.name} exchange issued no federated token`);
  assert.equal(token.token_type, "Bearer", `${app.name} federated token was not a bearer token`);
}

// The three assertMinted* flows prove the credential-minting console pages
// end to end from the operator's seat: the signed-in operator drives the real
// console UI, which calls the cloud's real credential APIs over the federated
// session, and the one-time secret disclosure behaves exactly as the real
// console's (shown once, gone after dismissal/reload). Whether the minted
// material then authenticates the vendor CLI is proven by the simulator
// CLI-test suites — this suite proves the console can mint it.

async function assertMintedIAMAccessKey(page, app) {
  const origin = new URL(app.launch).origin;
  const userName = `rps-minted-${Date.now() % 1_000_000}`;

  await page.goto(`${origin}/ui/iam`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("iam-create-user").click();
  await page.getByTestId("iam-user-name-input").fill(userName);
  await page.getByTestId("iam-create-user-submit").click();
  await page.getByRole("link", { name: userName }).click();

  await page.getByTestId("iam-create-access-key").click();
  await page.getByTestId("iam-create-access-key-submit").click();
  const keyId = (await page.getByTestId("iam-access-key-id").innerText()).trim();
  assert.match(keyId, /^AKIA/, `${app.name} minted access key id did not have the AKIA prefix`);
  await page.getByRole("button", { name: "Show" }).click();
  const secret = (await page.getByTestId("iam-secret-access-key").innerText()).trim();
  assert.ok(secret.length >= 30, `${app.name} revealed no secret access key`);
  await page.getByTestId("iam-cli-usage").waitFor({ state: "visible" });
  await page.getByTestId("iam-access-key-done").click();
  assert.equal(
    await page.getByText(secret).count(),
    0,
    `${app.name} secret access key remained visible after the one-time dialog closed`,
  );
}

async function assertMintedServiceAccountKey(page, app) {
  const origin = new URL(app.launch).origin;
  const accountId = `rps-minted-${Date.now() % 1_000_000}`;

  await page.goto(`${origin}/ui/serviceaccounts`, { waitUntil: "domcontentloaded" });
  await page.getByRole("button", { name: "Create service account" }).first().click();
  await page.getByTestId("sa-create-name").fill(accountId);
  await page.getByTestId("sa-create-submit").click();
  await page.locator(`[data-testid^="sa-row-${accountId}"]`).click();

  await page.getByTestId("sa-keys-add").click();
  await page.getByTestId("sa-key-create-submit").click();
  await page.getByTestId("sa-key-minted-dialog").waitFor({ state: "visible" });
  const filename = (await page.getByTestId("sa-key-filename").innerText()).trim();
  assert.match(filename, /\.json$/, `${app.name} minted key filename was not a JSON key file`);
  const href = await page.getByTestId("sa-key-download").getAttribute("href");
  assert.ok(href?.startsWith("data:"), `${app.name} minted key download was not a self-contained data URI`);
  await page.getByTestId("sa-key-cli").waitFor({ state: "visible" });
  await page.getByTestId("sa-key-minted-done").click();
  assert.equal(
    await page.getByTestId("sa-key-download").count(),
    0,
    `${app.name} private key download remained available after the one-time dialog closed`,
  );
}

async function assertMintedEntraClientSecret(page, app) {
  const origin = new URL(app.launch).origin;
  const appName = `rps-minted-${Date.now() % 1_000_000}`;

  await page.goto(`${origin}/ui/entra/app-registrations`, { waitUntil: "domcontentloaded" });
  await page.getByRole("button", { name: "New registration" }).click();
  await page.getByTestId("entra-app-name-input").fill(appName);
  await page.getByTestId("entra-register-submit").click();

  const clientId = (await page.getByTestId("entra-app-client-id").innerText()).trim();
  assert.match(
    clientId,
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i,
    `${app.name} registration exposed no application (client) ID`,
  );
  await page.getByTestId("entra-new-client-secret").click();
  await page.getByTestId("entra-secret-description").fill("rps-proof");
  await page.getByTestId("entra-secret-add").click();
  const secret = (await page.getByTestId("entra-secret-value").innerText()).trim();
  assert.ok(secret.length >= 20, `${app.name} minted client secret was empty`);
  await page.getByTestId("entra-secret-notice").waitFor({ state: "visible" });
  await page.getByTestId("entra-cli-usage").waitFor({ state: "visible" });

  await page.reload({ waitUntil: "domcontentloaded" });
  assert.equal(
    await page.getByText(secret).count(),
    0,
    `${app.name} client secret was still readable after leaving the blade`,
  );
  await page.getByTestId("entra-secret-hint").first().waitFor({ state: "visible" });
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
  await page.waitForLoadState("domcontentloaded");
  // The identity marker is always visible. Sign-out may sit inside an account
  // menu behind the avatar, the way the real cloud console arranges it, so it
  // is not present until the menu is opened — logout opens it and clicks it.
  await page.locator("[data-shauth-user]").waitFor({ state: "visible" });
}

async function waitForShauthLogin(page) {
  await page.waitForURL((raw) => {
    const target = new URL(raw.toString());
    return target.origin === authOrigin && target.pathname === "/login";
  }, { timeout: 30_000 });
}

async function assertIdentity(page, context, app) {
  const response = await context.request.get(app.identity);
  assert.equal(response.status(), 200, `${app.name} identity endpoint returned ${response.status()}`);
  const identity = await response.json();
  assert.equal(identity.authenticated, true, `${app.name} did not expose an authenticated identity`);
  const expectedName = identity.name || identity.preferred_username || identity.preferredUsername || identity.login || identity.user || identity.email || identity.sub;
  assert.equal(typeof expectedName, "string", `${app.name} identity omitted every user-facing name`);
  assert.notEqual(expectedName.trim(), "", `${app.name} identity exposed an empty user-facing name`);
  const account = page.locator("[data-shauth-user]");
  await account.waitFor({ state: "visible" });
  assert.equal(await account.getAttribute("data-shauth-user"), expectedName, `${app.name} product UI exposed the wrong Shauth username marker`);
  // The signed-in name and sign-out may live inside an account menu behind the
  // avatar, the way the real cloud console arranges them; open it before
  // asserting they are shown.
  await openAccountMenu(page);
  assert.match(await account.innerText(), new RegExp(escapeRegExp(expectedName), "i"), `${app.name} product UI did not show the authenticated user`);
  await page.locator("[data-shauth-sign-out]").waitFor({ state: "visible" });
}

// openAccountMenu reveals the account menu when the console tucks the identity
// and sign-out behind an avatar. It is idempotent: a console that shows them
// directly has no trigger, and one already open is left open.
async function openAccountMenu(page) {
  const trigger = page.locator("[data-shauth-account-trigger]");
  if ((await trigger.count()) && (await trigger.getAttribute("aria-expanded")) === "false") {
    await trigger.click();
  }
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
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
