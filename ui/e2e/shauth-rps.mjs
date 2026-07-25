import assert from "node:assert/strict";
import { spawn, execFile } from "node:child_process";
import { existsSync } from "node:fs";
import { mkdir } from "node:fs/promises";
import { promisify } from "node:util";
import { chromium } from "playwright";

const execFileAsync = promisify(execFile);

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
      await assertCreatedProject(page);
    }
    if (app.name === "Sockerless AWS simulator") {
      await assertFederatedAwsCredentials(context, page, app);
      await assertMintedIAMAccessKey(page, app);
      await assertCreatedOrganizationAccount(page, app);
    }
    if (app.name === "Sockerless Microsoft Azure simulator") {
      await assertFederatedAzureToken(context, page, app);
      await assertMintedEntraClientSecret(page, app);
    }
    await logoutFromApplication(page, context, app);
    assert.deepEqual(failures, [], `${app.name} direct login emitted browser failures`);
    await context.close();
  }

  await assertSockerlessCliLogin(browser);

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
// operator through the console's own server-side broker — the fix for the
// cross-origin gap that once blocked the Azure browser data plane. Real
// Microsoft Entra serves no CORS for the client_credentials grant, so the
// console (its own OpenID Connect relying party) performs the Workload Identity
// Federation exchange server-side at /auth/federation/token, against a separate
// cloud process whose federated identity credential the harness provisioned for
// the operator's subject; the browser then reads Azure Resource Manager and
// Microsoft Graph directly over that cloud's real CORS. A bearer token here
// means the whole server-side federation works end to end with a live identity.
async function assertFederatedAzureToken(context, page, app) {
  const origin = new URL(app.launch).origin;
  for (const scope of ["arm", "graph"]) {
    const brokered = await context.request.get(`${origin}/auth/federation/token?scope=${scope}`);
    assert.equal(brokered.status(), 200, `${app.name} federation broker (${scope}) returned ${brokered.status()}`);
    const token = await brokered.json();
    assert.ok(token.access_token, `${app.name} federation broker issued no ${scope} token`);
    assert.equal(token.token_type, "Bearer", `${app.name} brokered ${scope} token was not a bearer token`);
  }
  // The signed-in console reads a real cloud API through that federation; a load
  // error would mean the SPA's broker-backed federation path is broken.
  await page.goto(`${origin}/ui/subscriptions`, { waitUntil: "domcontentloaded" });
  await page.getByRole("heading", { name: "Subscriptions" }).waitFor({ state: "visible" });
  assert.equal(
    await page.getByText("Couldn't load").count() + await page.getByText("Could not load").count(),
    0,
    `${app.name} portal failed to read the real Azure API over the federation broker`,
  );
}
// assertSockerlessCliLogin proves the packaged terminal sign-in end to end:
// `sockerless login` starts the RFC 8252 loopback flow, the operator signs
// into Shauth (and authorizes the CLI once) in a real browser, and the vendor
// tools then work with the credentials the CLI wired — the AWS CLI assumes the
// federation role itself via the written profile, az re-exchanges its stored
// federated assertion, and gcloud uses the workforce external-account file.
// `sockerless logout` then removes what login wrote.
async function assertSockerlessCliLogin(browser) {
  const bin = process.env.SOCKERLESS_CLI_BIN;
  assert.ok(bin, "SOCKERLESS_CLI_BIN is required (the harness builds and exports the CLI)");
  const cliEnv = {
    ...process.env,
    // A dedicated HOME isolates every vendor tool's implicit per-user state —
    // most importantly the AWS CLI's assume-role cache (~/.aws/cli/cache),
    // which has no env override and is keyed such that a credential cached by
    // an earlier run against an earlier simulator instance would be replayed
    // against this one and rejected. CI runners start clean; a local run must
    // behave identically.
    HOME: `${process.env.SOCKERLESS_CLI_HOME}/home`,
    SOCKERLESS_HOME: process.env.SOCKERLESS_CLI_HOME,
    SOCKERLESS_CONTEXT: "rps",
    AWS_CONFIG_FILE: process.env.SOCKERLESS_CLI_AWS_CONFIG_FILE,
    CLOUDSDK_CONFIG: process.env.SOCKERLESS_CLI_CLOUDSDK_CONFIG,
    CLOUDSDK_ACTIVE_CONFIG_NAME: "sockerless-rps",
    AZURE_CONFIG_DIR: process.env.SOCKERLESS_CLI_AZURE_CONFIG_DIR,
    REQUESTS_CA_BUNDLE: process.env.SOCKERLESS_CLI_AZURE_CA_BUNDLE,
  };
  await mkdir(cliEnv.HOME, { recursive: true });

  const child = spawn(bin, ["login", "--no-browser", "--timeout", "180s"], { env: cliEnv });
  let output = "";
  const urlPromise = new Promise((resolve, reject) => {
    child.stdout.on("data", (chunk) => {
      output += chunk;
      const match = output.match(/http:\/\/localhost:8080\/oauth2\/auth\?\S+/);
      if (match) resolve(match[0]);
    });
    child.on("error", reject);
  });
  let errors = "";
  child.stderr.on("data", (chunk) => {
    errors += chunk;
  });
  const exitPromise = new Promise((resolve) => child.on("close", resolve));

  const authorizeURL = await urlPromise;
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(authorizeURL, { waitUntil: "domcontentloaded" });
  await signInIfRequired(page);
  // A terminal client is not a Shauth-managed application, so the first
  // authorization presents Shauth's explicit consent screen.
  const authorize = page.getByRole("button", { name: "Authorize application" });
  if (await authorize.isVisible().catch(() => false)) {
    await authorize.click();
  }
  await page.getByRole("heading", { name: "Signed in" }).waitFor({ state: "visible" });
  const exitCode = await exitPromise;
  assert.equal(exitCode, 0, `sockerless login exited ${exitCode}\nstdout:\n${output}\nstderr:\n${errors}`);
  await context.close();

  // The AWS CLI performs AssumeRoleWithWebIdentity itself from the written
  // profile — the credential path is entirely vendor-native.
  let aws;
  try {
    aws = await execFileAsync("aws", ["--profile", "sockerless-rps", "sts", "get-caller-identity", "--output", "json"], { env: cliEnv });
  } catch (cause) {
    const debug = await execFileAsync("aws", ["--profile", "sockerless-rps", "sts", "get-caller-identity", "--debug"], { env: cliEnv }).catch((e) => e);
    throw new Error(
      `aws get-caller-identity failed: ${cause.stderr ?? cause}\n--- debug tail ---\n${String(debug.stderr ?? "").split("\n").slice(-60).join("\n")}`,
      { cause },
    );
  }
  const awsIdentity = JSON.parse(aws.stdout);
  assert.match(awsIdentity.Arn ?? "", /cli-federation-role/, `aws sts get-caller-identity returned ${aws.stdout}`);

  // az re-exchanges the stored federated assertion on demand.
  const az = await execFileAsync("az", ["account", "show", "--output", "json"], { env: cliEnv });
  assert.ok(JSON.parse(az.stdout).id, `az account show returned ${az.stdout}`);

  // gcloud resolves the workforce identity through the real STS exchange and
  // introspection from the external-account credential file.
  const gcloud = await execFileAsync("gcloud", ["iam", "service-accounts", "list", "--format", "json"], { env: cliEnv });
  assert.ok(Array.isArray(JSON.parse(gcloud.stdout)), `gcloud service-accounts list returned ${gcloud.stdout}`);

  const logout = await execFileAsync(bin, ["logout"], { env: cliEnv });
  const tokenPath = `${process.env.SOCKERLESS_CLI_HOME}/contexts/rps/web-identity-token`;
  assert.ok(!existsSync(tokenPath), `sockerless logout left the web identity token behind\n${logout.stdout}`);
}

// The assertMinted* flows prove the credential-minting console pages
// end to end from the operator's seat: the signed-in operator drives the real
// console UI, which calls the cloud's real credential APIs over the federated
// session, and the one-time secret disclosure behaves exactly as the real
// console's (shown once, gone after dismissal/reload). Whether the minted
// material then authenticates the vendor CLI is proven by the simulator
// CLI-test suites — this suite proves the console can mint it.

async function assertMintedIAMAccessKey(page, app) {
  const userName = `rps-minted-${Date.now() % 1_000_000}`;

  // Navigate through the console's own navigation, as an operator does. An SPA
  // route change (unlike page.goto) does not tear the document down, so the
  // previous page's in-flight API reads settle instead of aborting — a full
  // navigation here raced the prior page's reads and tripped the monitor.
  await page.getByRole("link", { name: "Identity and Access Management" }).click();
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
  // The key list refetches after the dialog closes; waiting for the minted key
  // id to appear proves the refetch settled (so no in-flight request is aborted
  // by the next navigation) and that the key is listed as metadata only.
  await page.getByText(keyId).waitFor({ state: "visible" });
  assert.equal(
    await page.getByText(secret).count(),
    0,
    `${app.name} secret access key remained visible after the one-time dialog closed`,
  );
}

async function assertMintedServiceAccountKey(page, app) {
  const accountId = `rps-minted-${Date.now() % 1_000_000}`;

  // Console-native SPA navigation — see assertMintedIAMAccessKey.
  await page.getByRole("link", { name: "Service accounts" }).click();
  await page.getByRole("button", { name: "Create service account" }).first().click();
  await page.getByTestId("sa-create-name").fill(accountId);
  await page.getByTestId("sa-create-submit").click();
  await page.locator(`[data-testid^="sa-row-${accountId}"]`).click();

  await page.getByTestId("sa-keys-add").click();
  await page.getByTestId("sa-key-create-submit").click();
  await page.getByTestId("sa-key-minted-dialog").waitFor({ state: "visible" });
  // The element carries the filename plus the real console's store-it-securely
  // sentence; assert the project-prefixed JSON key filename within it.
  const filename = (await page.getByTestId("sa-key-filename").innerText()).trim();
  assert.match(filename, /sockerless-[0-9a-f]+\.json/, `${app.name} minted key filename was not a JSON key file`);
  const href = await page.getByTestId("sa-key-download").getAttribute("href");
  assert.ok(href?.startsWith("data:"), `${app.name} minted key download was not a self-contained data URI`);
  await page.getByTestId("sa-key-cli").waitFor({ state: "visible" });
  await page.getByTestId("sa-key-minted-done").click();
  // Wait for the minted key's row so the post-create refetch settles before the
  // suite navigates on (an in-flight request aborted by navigation would trip
  // the browser-failure monitor).
  await page.locator('[data-testid^="sa-key-delete-"]').first().waitFor({ state: "visible" });
  assert.equal(
    await page.getByTestId("sa-key-download").count(),
    0,
    `${app.name} private key download remained available after the one-time dialog closed`,
  );
}


// assertCreatedOrganizationAccount drives the AWS Organizations console page
// as the signed-in operator: create the organization when none exists, then
// add an AWS account through the real console flow — the asynchronous
// CreateAccount request polled to SUCCEEDED — and see the account listed.
async function assertCreatedOrganizationAccount(page, app) {
  const accountName = `rps-account-${Date.now() % 1_000_000}`;

  await page.getByRole("link", { name: "AWS Organizations" }).click();
  const createOrg = page.getByTestId("org-create-organization");
  const addAccount = page.getByTestId("org-add-account");
  await createOrg.or(addAccount).first().waitFor({ state: "visible" });
  if (await createOrg.isVisible()) {
    await createOrg.click();
  }
  await addAccount.click();
  await page.getByTestId("org-account-name-input").fill(accountName);
  await page.getByTestId("org-account-email-input").fill(`${accountName}@example.com`);
  await page.getByTestId("org-add-account-submit").click();

  const createdId = (await page.getByTestId("org-created-account-id").innerText()).trim();
  assert.match(createdId, /^\d{12}$/, `${app.name} account creation did not report a 12-digit account id`);
  await page.getByTestId("org-add-account-done").click();
  await page.getByTestId(`org-account-row-${createdId}`).waitFor({ state: "visible" });
}

// assertCreatedProject drives the Google Cloud console's project picker as the
// signed-in operator: open the picker, create a project through the real
// Resource Manager create + long-running operation, and see the console switch
// to it (real-console behavior after New Project completes).
async function assertCreatedProject(page) {
  const projectId = `rps-project-${Date.now() % 1_000_000}`;

  await page.getByTestId("project-picker").click();
  await page.getByTestId("project-dialog").waitFor({ state: "visible" });
  await page.getByTestId("project-create-open").click();
  await page.getByTestId("project-create-name").fill(projectId);
  await page.getByTestId("project-create-id").fill(projectId);
  await page.getByTestId("project-create-submit").click();

  // The picker chip reflects the selected project once the create operation
  // completes and the dialog selects the new project.
  await page.getByTestId("project-picker").getByText(projectId).waitFor({ state: "visible" });

  // Switch back to the seeded project so the remainder of the iteration
  // (logout and later loops) runs against the default coordinates.
  await page.getByTestId("project-picker").click();
  await page.getByTestId("project-row-sockerless").click();
  await page.getByTestId("project-picker").getByText("sockerless").waitFor({ state: "visible" });
}

// assertMintedEntraClientSecret drives the Azure portal's app-registration and
// Certificates & secrets blades as the signed-in operator — the browser data
// plane the server-side federation broker unblocks. It creates an app
// registration and mints a client secret through the real Microsoft Graph
// APIs, and asserts the real portal's one-time secret disclosure.
async function assertMintedEntraClientSecret(page, app) {
  const appName = `rps-minted-${Date.now() % 1_000_000}`;

  await page.getByRole("link", { name: "App registrations" }).click();
  await page.getByRole("button", { name: "New registration" }).click();
  await page.getByTestId("entra-app-name-input").fill(appName);
  await page.getByTestId("entra-register-submit").click();

  let clientId;
  try {
    clientId = (await page.getByTestId("entra-app-client-id").innerText()).trim();
  } catch (cause) {
    const alerts = await page.getByRole("alert").allInnerTexts();
    throw new Error(
      `${app.name} registration detail never rendered at ${page.url()}; visible alerts: ${JSON.stringify(alerts)}`,
      { cause },
    );
  }
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
  // Wait for the stored-credential row so the post-mint refetch settles before
  // the reload (an aborted in-flight request would trip the failure monitor).
  await page.getByTestId("entra-secret-row").first().waitFor({ state: "visible" });

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
    // ERR_ABORTED is Chromium's cancellation code, not a network failure: a
    // navigation tearing down the document cancels the page's in-flight reads,
    // which is healthy single-page-application behavior, and the suite's
    // gotos race those reads nondeterministically. Real failures — refused
    // connections, DNS errors, error documents, page errors — stay flagged.
    if (reason === "net::ERR_ABORTED") return;
    failures.push(`request failed (${reason}): ${new URL(request.url()).origin}${new URL(request.url()).pathname}`);
  });
  page.on("response", (response) => {
    if (response.request().isNavigationRequest() && response.status() >= 400 && !allowedDocumentStatuses.has(response.status())) {
      failures.push(`document ${response.status()}: ${new URL(response.url()).origin}${new URL(response.url()).pathname}`);
    }
  });
  return failures;
}
