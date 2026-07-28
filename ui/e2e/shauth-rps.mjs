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
      await assertCreatedGCSBucket(page, app);
      await assertRanCloudRunJob(page, app);
      await assertCreatedPubSubTopic(page, app);
    }
    if (app.name === "Sockerless AWS simulator") {
      await assertFederatedAwsCredentials(context, page, app);
      await assertMintedIAMAccessKey(page, app);
      await assertCreatedOrganizationAccount(page, app);
      await assertCreatedECRRepository(page, app);
      await assertCreatedDynamoDBTable(page, app);
      await assertManagedLambdaFunction(page, app);
      await assertRanStepFunctionsWorkflow(page, app);
      await assertManagedAwsEventingAndObservability(page, app);
      await assertManagedFirehoseAndPrivateCA(page, app);
      await assertManagedAmplifyAndRDS(page, app);
    }
    if (app.name === "Sockerless Microsoft Azure simulator") {
      await assertFederatedAzureToken(context, page, app);
      await assertMintedEntraClientSecret(page, app);
      await assertResourceDetailBladeRendersLiveData(page, app);
      await assertCreatedACRRegistry(page, app);
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


// assertCreatedDynamoDBTable proves a service the console had wrongly advertised
// as unsupported is genuinely usable by the signed-in operator: create a table
// through the real DynamoDB CreateTable API over the federated SigV4 path, see
// it in the list, then delete it through DeleteTable and see it leave.
async function assertCreatedDynamoDBTable(page, app) {
  const origin = new URL(app.launch).origin;
  const tableName = `rps-table-${Date.now() % 1_000_000}`;
  await page.goto(`${origin}/ui/dynamodb`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("dynamodb-create-table").click();
  await page.getByTestId("dynamodb-table-name-input").fill(tableName);
  await page.getByTestId("dynamodb-partition-key-input").fill("id");
  await page.getByTestId("dynamodb-create-table-submit").click();
  await page.getByRole("link", { name: tableName }).waitFor({ state: "visible" });

  await page.getByRole("link", { name: tableName }).click();
  await page.getByTestId("dynamodb-table-delete").click();
  await page.getByTestId("dynamodb-delete-table-confirm").click();
  await page.goto(`${origin}/ui/dynamodb`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("dynamodb-create-table").waitFor({ state: "visible" });
  assert.equal(
    await page.getByRole("link", { name: tableName }).count(),
    0,
    `${app.name} deleted DynamoDB table still appears in the list`,
  );
}

// assertCreatedPubSubTopic proves the same for Google Cloud: Pub/Sub was
// advertised as unsupported, and the operator can create a topic through the
// real topics.create API and see it listed.
async function assertCreatedPubSubTopic(page, app) {
  const origin = new URL(app.launch).origin;
  const topicId = `rps-topic-${Date.now() % 1_000_000}`;
  await page.goto(`${origin}/ui/pubsub`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("pubsub-create-topic").click();
  await page.getByTestId("pubsub-create-topic-id").fill(topicId);
  await page.getByTestId("pubsub-create-topic-submit").click();
  await page.getByRole("link", { name: topicId }).waitFor({ state: "visible" });
}

// assertRanCloudRunJob proves the Google Cloud console's compute-create and
// lifecycle-action flows end to end for the signed-in operator: create a Cloud
// Run job through the real jobs.create API, then Run it (real jobs.run, which
// creates an execution). Both are long-running operations the console drives
// through real operations.get polling.
async function assertRanCloudRunJob(page, app) {
  const origin = new URL(app.launch).origin;
  const jobId = `rps-job-${Date.now() % 1_000_000}`;
  await page.goto(`${origin}/ui/cloudrun`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("cloudrun-create-job").click();
  await page.getByTestId("cloudrun-create-id").fill(jobId);
  await page.getByTestId("cloudrun-create-image").fill("alpine:latest");
  await page.getByTestId("cloudrun-create-submit").click();
  await page.getByRole("link", { name: jobId }).waitFor({ state: "visible" });

  // Run the created job from its detail page; the confirm dialog closing without
  // error confirms the real jobs.run operation completed.
  await page.getByRole("link", { name: jobId }).click();
  await page.getByTestId("cloudrun-job-run").click();
  await page.getByTestId("cloudrun-run-confirm").click();
  await page.getByTestId("cloudrun-run-confirm").waitFor({ state: "detached" });
}

// assertCreatedGCSBucket proves the Google Cloud console's create flow end to
// end for the signed-in operator: open Cloud Storage, create a bucket through
// the real storage.buckets.insert API over the federated bearer path, and see
// it appear in the list.
async function assertCreatedGCSBucket(page, app) {
  const origin = new URL(app.launch).origin;
  const bucketName = `rps-bucket-${Date.now() % 1_000_000}`;
  await page.goto(`${origin}/ui/gcs`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("gcs-create-bucket").click();
  await page.getByTestId("gcs-create-name").fill(bucketName);
  await page.getByTestId("gcs-create-submit").click();
  await page.getByRole("link", { name: bucketName }).waitFor({ state: "visible" });

  // Delete the bucket through its detail page and confirm it leaves the list —
  // the create → delete round trip through the real storage.buckets APIs.
  await page.getByRole("link", { name: bucketName }).click();
  await page.getByTestId("gcs-bucket-delete").click();
  await page.getByTestId("gcs-delete-confirm").click();
  await page.goto(`${origin}/ui/gcs`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("gcs-create-bucket").waitFor({ state: "visible" });
  assert.equal(
    await page.getByRole("link", { name: bucketName }).count(),
    0,
    `${app.name} deleted Cloud Storage bucket still appears in the list`,
  );
}

// assertCreatedACRRegistry proves the Azure portal's create flow end to end for
// the signed-in operator: open Container registries, create a registry through
// the real Azure Resource Manager PUT over the federated broker path, and see it
// appear in the list. The name is the only required field — subscription,
// resource group, location, and SKU carry defaults.
async function assertCreatedACRRegistry(page, app) {
  const origin = new URL(app.launch).origin;
  const registryName = `rpsreg${Date.now() % 1_000_000}`;
  await page.goto(`${origin}/ui/acr`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("acr-create").click();
  await page.getByTestId("acr-create-name").fill(registryName);
  await page.getByTestId("acr-create-submit").click();
  await page.getByRole("link", { name: registryName }).waitFor({ state: "visible" });

  // Open the registry's detail blade and edit it (Update) — toggle admin-user
  // through the real ARM PATCH; the dialog closing confirms the write.
  await page.getByRole("link", { name: registryName }).click();
  await page.getByTestId("acr-registry-update").click();
  await page.getByTestId("acr-update-admin").click();
  await page.getByTestId("acr-update-save").click();
  await page.getByTestId("acr-update-save").waitFor({ state: "detached" });

  // Delete the registry from the same detail blade and confirm it leaves the
  // list — completing the create → update → delete round trip through the real
  // Azure Resource Manager container-registry APIs.
  await page.getByTestId("acr-registry-delete").click();
  await page.getByTestId("acr-registry-delete-dialog-confirm").click();
  await page.goto(`${origin}/ui/acr`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("acr-create").waitFor({ state: "visible" });
  assert.equal(
    await page.getByRole("link", { name: registryName }).count(),
    0,
    `${app.name} deleted container registry still appears in the list`,
  );
}

// assertCreatedECRRepository proves the AWS console's resource-creation flow end
// to end for the signed-in operator: open Elastic Container Registry, create a
// repository through the real ECR CreateRepository API over the federated SigV4
// path, and see it appear in the list. This exercises an authenticated
// create → list-refresh round trip the per-package e2e cannot (that suite has
// no identity provider, so the simulator rejects its unsigned writes with 403).
async function assertCreatedECRRepository(page, app) {
  const origin = new URL(app.launch).origin;
  const repoName = `rps-repo-${Date.now() % 1_000_000}`;
  await page.goto(`${origin}/ui/ecr`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("ecr-create-repo").click();
  await page.getByTestId("ecr-repo-name-input").fill(repoName);
  await page.getByTestId("ecr-create-repo-submit").click();
  // The created repository appears in the list; its name column is a link to the
  // repository detail.
  await page.getByRole("link", { name: repoName }).waitFor({ state: "visible" });

  // Edit the repository's settings (Update) — toggle scan-on-push through the
  // real PutImageScanningConfiguration API; the dialog closing without error
  // confirms the authenticated write round-tripped.
  await page.getByRole("link", { name: repoName }).click();
  await page.getByTestId("ecr-repo-edit").click();
  await page.getByTestId("ecr-scan-toggle").click();
  await page.getByTestId("ecr-repo-settings-save").click();
  await page.getByTestId("ecr-repo-settings-save").waitFor({ state: "detached" });
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
  // The created account appears in the accounts table; its name column is a link
  // to the account detail (the Cloudscape table renders no per-row test id).
  await page.getByRole("link", { name: accountName }).waitFor({ state: "visible" });
}

// assertManagedLambdaFunction drives the resource sub-surfaces the AWS Lambda
// console exposes through Lambda's public REST-JSON API. It creates a real
// function resource, updates its code and configuration, publishes and aliases
// a version, and configures the trigger, asynchronous destination, provisioned
// concurrency, and function URL. Every read after a write comes from Lambda's
// API over the operator's federated SigV4 credentials.
async function assertManagedLambdaFunction(page, app) {
  const origin = new URL(app.launch).origin;
  const functionName = `rps-lambda-${Date.now() % 1_000_000}`;
  await page.goto(`${origin}/ui/lambda`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("lambda-create-function").click();
  const create = page.getByRole("dialog", { name: "Create function" });
  await create.getByTestId("lambda-function-name-input").fill(functionName);
  await create.getByTestId("lambda-image-uri-input").fill("123456789012.dkr.ecr.us-east-1.amazonaws.com/rps:v1");
  await create
    .getByTestId("lambda-role-input")
    .fill("arn:aws:iam::123456789012:role/console-federation-role");
  await create.getByTestId("lambda-create-function-submit").click();
  await create.waitFor({ state: "detached" });
  await page.getByRole("link", { name: functionName, exact: true }).click();
  await page.getByTestId("lambda-function-summary").waitFor({ state: "visible" });

  await page.getByTestId("lambda-function-edit").click();
  const edit = page.getByRole("dialog", { name: "Edit basic settings" });
  await edit.getByTestId("lambda-edit-config-memory").fill("512");
  await edit.getByTestId("lambda-edit-config-timeout").fill("30");
  await edit.getByTestId("lambda-edit-config-description").fill("managed from the console");
  await edit.getByTestId("lambda-edit-config-save").click();
  await edit.waitFor({ state: "detached" });

  await page.getByRole("tab", { name: "Code" }).click();
  await page.getByRole("button", { name: "Upload code" }).click();
  const code = page.getByRole("dialog", { name: "Upload code" });
  await code.getByLabel("Container image URI").fill("123456789012.dkr.ecr.us-east-1.amazonaws.com/rps:v2");
  await code.getByRole("button", { name: "Save" }).click();
  await code.waitFor({ state: "detached" });

  await page.getByRole("button", { name: "Publish version" }).click();
  const publish = page.getByRole("dialog", { name: "Publish new version" });
  await publish.getByLabel("Version description").fill("browser validated");
  await publish.getByRole("button", { name: "Publish" }).click();
  await publish.waitFor({ state: "detached" });

  await page.getByRole("tab", { name: /Aliases/ }).click();
  await page.getByRole("button", { name: "Create alias" }).click();
  const alias = page.getByRole("dialog", { name: "Create alias" });
  await alias.getByLabel("Alias name").fill("live");
  await alias.getByLabel("Function version").click();
  await page.getByRole("option", { name: "1", exact: true }).click();
  await alias.getByRole("button", { name: "Create" }).click();
  await alias.waitFor({ state: "detached" });
  await page.getByText("live", { exact: true }).waitFor({ state: "visible" });

  await page.getByRole("tab", { name: "Monitor" }).click();
  await page.getByRole("button", { name: "Add configuration" }).click();
  const provisioned = page.getByRole("dialog", { name: "Provisioned concurrency configuration" });
  await provisioned.getByLabel("Alias or version").click();
  await page.getByRole("option", { name: "live", exact: true }).click();
  await provisioned.getByLabel("Provisioned concurrent executions").fill("2");
  await provisioned.getByRole("button", { name: "Save" }).click();
  await provisioned.waitFor({ state: "detached" });
  await page.getByText("READY", { exact: true }).waitFor({ state: "visible" });

  await page.getByRole("tab", { name: /Event source mappings/ }).click();
  await page.getByRole("button", { name: "Add event source mapping" }).click();
  const eventSource = page.getByRole("dialog", { name: "Add event source mapping" });
  await eventSource
    .getByLabel("Event source ARN")
    .fill("arn:aws:kinesis:us-east-1:123456789012:stream/rps-events");
  await eventSource.getByRole("button", { name: "Save" }).click();
  await eventSource.waitFor({ state: "detached" });
  await page.getByText("rps-events", { exact: false }).waitFor({ state: "visible" });

  await page.getByRole("tab", { name: /Asynchronous invocation/ }).click();
  await page.getByRole("button", { name: "Add configuration" }).click();
  const asynchronous = page.getByRole("dialog", { name: "Configure asynchronous invocation" });
  await asynchronous
    .getByLabel("On failure destination ARN")
    .fill("arn:aws:sqs:us-east-1:123456789012:rps-failures");
  await asynchronous.getByRole("button", { name: "Save" }).click();
  await asynchronous.waitFor({ state: "detached" });
  await page.getByText("rps-failures", { exact: false }).waitFor({ state: "visible" });

  await page.getByRole("tab", { name: "Function URL" }).click();
  await page.getByRole("button", { name: "Create function URL" }).click();
  const functionURL = page.getByRole("dialog", { name: "Create function URL" });
  await functionURL.getByRole("button", { name: "Create" }).click();
  await functionURL.waitFor({ state: "detached" });
  await page.getByText(/lambda-url\.us-east-1\.on\.aws/).waitFor({ state: "visible" });

  await page.getByRole("tab", { name: "Function overview" }).click();
  await page.getByText("On failure", { exact: true }).waitFor({ state: "visible" });
  assert.equal(
    await page.getByTestId("lambda-function-error").count(),
    0,
    `${app.name} AWS Lambda detail failed during its live management round trip`,
  );
}

// assertRanStepFunctionsWorkflow drives the same signed-in AWS console data
// plane used against real AWS: create a state machine, inspect its graph, start
// an execution, and read the completed execution history and output through the
// public AWS Step Functions APIs.
async function assertRanStepFunctionsWorkflow(page, app) {
  const origin = new URL(app.launch).origin;
  const machineName = `rps-workflow-${Date.now() % 1_000_000}`;

  await page.goto(`${origin}/ui/stepfunctions`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("sfn-create-state-machine").click();
  const dialog = page.getByRole("dialog", { name: "Create state machine" });
  await dialog.getByLabel("State machine name").fill(machineName);
  await dialog
    .getByLabel("Execution role ARN")
    .fill("arn:aws:iam::123456789012:role/console-federation-role");
  await dialog.getByTestId("sfn-create-state-machine-submit").click();

  await page.getByTestId("sfn-state-machine-summary").waitFor({ state: "visible" });
  await page.getByText("Pass", { exact: true }).first().waitFor({ state: "visible" });
  assert.equal(
    await page.getByTestId("sfn-state-machine-error").count(),
    0,
    `${app.name} state machine detail failed to read the live definition`,
  );

  await page.getByRole("button", { name: "Test state" }).click();
  const testState = page.getByRole("dialog", { name: "Test state" });
  await testState.getByLabel("State input").fill('{"probe":true}');
  await testState.getByRole("button", { name: "Test state" }).click();
  await testState.getByText("SUCCEEDED", { exact: true }).waitFor({ state: "visible" });
  assert.match(
    await testState.innerText(),
    /Hello from AWS Step Functions/,
    `${app.name} TestState did not return the live state output`,
  );
  await testState.getByRole("button", { name: "Close", exact: true }).last().click();

  for (const description of ["first browser version", "second browser version"]) {
    await page.getByRole("button", { name: "Publish version" }).click();
    const publish = page.getByRole("dialog", { name: "Publish version" });
    await publish.getByLabel("Version description").fill(description);
    await publish.getByRole("button", { name: "Publish" }).click();
    await publish.waitFor({ state: "detached" });
  }
  await page.getByRole("tab", { name: /Aliases/ }).click();
  await page.getByRole("button", { name: "Create alias" }).click();
  const createAlias = page.getByRole("dialog", { name: "Create alias" });
  await createAlias.getByLabel("Alias name").fill("production");
  await createAlias.getByLabel("State machine version").click();
  await page.getByRole("option", { name: "2", exact: true }).click();
  await createAlias.getByRole("button", { name: "Create" }).click();
  await createAlias.waitFor({ state: "detached" });
  await page.getByText("production", { exact: true }).waitFor({ state: "visible" });
  await page
    .getByRole("row", { name: /production/ })
    .getByRole("button", { name: "Edit", exact: true })
    .click();
  const editAlias = page.getByRole("dialog", { name: "Edit production" });
  await editAlias.getByLabel("Optional second version").click();
  await page.getByRole("option", { name: "1", exact: true }).click();
  await editAlias.getByLabel("Primary version traffic weight").fill("80");
  await editAlias.getByRole("button", { name: "Save" }).click();
  await editAlias.waitFor({ state: "detached" });
  await page.getByText(/2 \(80%\).*1 \(20%\)/).waitFor({ state: "visible" });

  await page.getByTestId("sfn-state-machine-start").click();
  await page.getByTestId("sfn-execution-input").locator("textarea").fill('{"order":"A-100"}');
  await page.getByTestId("sfn-start-execution-submit").click();

  await page.getByText("SUCCEEDED", { exact: true }).first().waitFor({ state: "visible" });
  await page.getByRole("tab", { name: /Table view/ }).click();
  await page.getByText("ExecutionSucceeded", { exact: true }).waitFor({ state: "visible" });
  await page.getByRole("tab", { name: "Input and output" }).click();
  const executionInput = page.getByTestId("sfn-execution-input-document").locator("textarea");
  const executionOutput = page.getByTestId("sfn-execution-output-document").locator("textarea");
  await executionInput.waitFor({ state: "visible" });
  assert.match(
    await executionInput.inputValue(),
    /"order": "A-100"/,
    `${app.name} execution detail did not render the live input`,
  );
  assert.match(
    await executionOutput.inputValue(),
    /Hello from AWS Step Functions/,
    `${app.name} execution detail did not render the live output`,
  );
}

// assertManagedAwsEventingAndObservability drives the connected AWS console
// workflows that surround Lambda and Step Functions. It creates and uses real
// Amazon SQS, Amazon SNS, EventBridge, EventBridge Scheduler, CloudWatch, and
// CloudTrail resources through the browser's federated SigV4 data plane.
async function assertManagedAwsEventingAndObservability(page, app) {
  const origin = new URL(app.launch).origin;
  const suffix = Date.now() % 1_000_000;
  const queueName = `rps-events-${suffix}`;
  const queueARN = `arn:aws:sqs:us-east-1:123456789012:${queueName}`;
  const topicName = `rps-events-${suffix}`;
  const eventBusName = `rps-bus-${suffix}`;
  const ruleName = `rps-events-${suffix}`;
  const scheduleName = `rps-events-${suffix}`;
  const alarmName = `rps-events-${suffix}`;
  const dashboardName = `rps-events-${suffix}`;
  const logGroupName = `/rps/events/${suffix}`;
  const trailName = `rps-events-${suffix}`;

  await page.goto(`${origin}/ui/sqs`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("sqs-create-queue").click();
  const createQueue = page.getByRole("dialog", { name: "Create queue" });
  await createQueue.getByTestId("sqs-queue-name-input").fill(queueName);
  await createQueue.getByTestId("sqs-create-queue-submit").click();
  await createQueue.waitFor({ state: "detached" });
  const queueRow = page.getByRole("row", { name: new RegExp(queueName) });
  await queueRow.getByRole("checkbox").click();
  await page.getByRole("button", { name: "Send and receive messages" }).click();
  const queue = page.getByRole("dialog", { name: queueName });
  await queue.getByLabel("Message body").fill("browser-to-sqs");
  await queue.getByRole("button", { name: "Send message" }).click();
  await queue.getByRole("button", { name: "Poll for messages" }).click();
  await queue.getByText("browser-to-sqs", { exact: true }).waitFor({ state: "visible" });
  await queue.getByRole("button", { name: "Delete message" }).click();
  await queue.getByText("browser-to-sqs", { exact: true }).waitFor({ state: "detached" });
  await queue.getByLabel("Access policy JSON").fill(JSON.stringify({
    Version: "2012-10-17",
    Statement: [{
      Effect: "Allow",
      Principal: { Service: ["sns.amazonaws.com", "events.amazonaws.com"] },
      Action: "sqs:SendMessage",
      Resource: queueARN,
    }],
  }));
  await queue.getByRole("button", { name: "Save access policy" }).click();
  await queue.getByText("Access policy saved.", { exact: true }).waitFor({ state: "visible" });
  await queue.getByRole("button", { name: "Close" }).last().click();

  await page.goto(`${origin}/ui/sns`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("sns-create-topic").click();
  const createTopic = page.getByRole("dialog", { name: "Create topic" });
  await createTopic.getByTestId("sns-topic-name-input").fill(topicName);
  await createTopic.getByTestId("sns-create-topic-submit").click();
  await createTopic.waitFor({ state: "detached" });
  const topicRow = page.getByRole("row", { name: new RegExp(topicName) });
  await topicRow.getByRole("checkbox").click();
  await page.getByRole("button", { name: "Publish and subscribe" }).click();
  const topic = page.getByRole("dialog", { name: topicName });
  await topic.getByLabel("Endpoint").fill(queueARN);
  await topic.getByRole("button", { name: "Create subscription" }).click();
  await topic.getByLabel("Subject").fill("Browser event");
  await topic.getByLabel("Message").fill("browser-to-sns");
  await topic.getByRole("button", { name: "Publish message" }).click();
  await page.waitForFunction(
    (element) => element.value === "",
    await topic.getByLabel("Message").elementHandle(),
  );
  assert.equal(await topic.getByLabel("Message").inputValue(), "", `${app.name} Amazon SNS publish did not complete`);
  await topic.getByRole("button", { name: "Close" }).last().click();
  await page.getByText(queueARN, { exact: true }).waitFor({ state: "visible" });

  await page.goto(`${origin}/ui/sqs`, { waitUntil: "domcontentloaded" });
  await page.getByRole("row", { name: new RegExp(queueName) }).getByRole("checkbox").click();
  await page.getByRole("button", { name: "Send and receive messages" }).click();
  const snsQueue = page.getByRole("dialog", { name: queueName });
  await snsQueue.getByRole("button", { name: "Poll for messages" }).click();
  await snsQueue.getByText(/browser-to-sns/).waitFor({ state: "visible" });
  await snsQueue.getByRole("button", { name: "Delete message" }).click();
  await snsQueue.getByRole("button", { name: "Close" }).last().click();

  await page.goto(`${origin}/ui/eventbridge`, { waitUntil: "domcontentloaded" });
  await page.getByRole("button", { name: "Create event bus", exact: true }).click();
  const createEventBus = page.getByRole("dialog", { name: "Create event bus" });
  await createEventBus.getByLabel("Name", { exact: true }).fill(eventBusName);
  await createEventBus.getByLabel("Description").fill("browser validated event bus");
  await createEventBus.getByRole("button", { name: "Create event bus" }).click();
  await createEventBus.waitFor({ state: "detached" });
  await page
    .getByTestId("eventbridge-buses-table")
    .getByText(eventBusName, { exact: true })
    .waitFor({ state: "visible" });
  await page.getByRole("button", { name: "Create rule", exact: true }).click();
  const createRule = page.getByRole("dialog", { name: "Create rule" });
  await createRule.getByLabel("Name", { exact: true }).fill(ruleName);
  await createRule.getByLabel("Description").fill("browser validated event routing");
  await createRule.getByLabel("Event bus name").fill(eventBusName);
  await createRule.getByRole("button", { name: "Create rule" }).click();
  await createRule.waitFor({ state: "detached" });
  const ruleRow = page.getByRole("row", { name: new RegExp(ruleName) });
  await ruleRow.getByRole("checkbox").click();
  await page.getByRole("button", { name: "Manage targets" }).click();
  const targets = page.getByRole("dialog", { name: `Targets for ${ruleName}` });
  await targets.getByLabel("Target ARN").fill(queueARN);
  await targets.getByRole("button", { name: "Add target" }).click();
  await targets.getByText(queueARN, { exact: true }).waitFor({ state: "visible" });
  await targets.getByRole("button", { name: "Close" }).last().click();
  await page.getByRole("button", { name: "Send event", exact: true }).click();
  const sendEvent = page.getByRole("dialog", { name: "Send event" });
  await sendEvent.getByLabel("Event bus name").fill(eventBusName);
  await sendEvent.getByRole("button", { name: "Send event" }).click();
  await sendEvent.getByText(/Event accepted with ID/).waitFor({ state: "visible" });
  await sendEvent.getByRole("button", { name: "Close" }).last().click();

  await page.goto(`${origin}/ui/sqs`, { waitUntil: "domcontentloaded" });
  await page.getByRole("row", { name: new RegExp(queueName) }).getByRole("checkbox").click();
  await page.getByRole("button", { name: "Send and receive messages" }).click();
  const eventQueue = page.getByRole("dialog", { name: queueName });
  await eventQueue.getByRole("button", { name: "Poll for messages" }).click();
  await eventQueue.getByText(/Application event/).waitFor({ state: "visible" });
  await eventQueue.getByRole("button", { name: "Delete message" }).click();
  await eventQueue.getByRole("button", { name: "Close" }).last().click();

  await page.goto(`${origin}/ui/scheduler`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("scheduler-schedules-table").getByRole("button", { name: "Create schedule", exact: true }).click();
  const createSchedule = page.getByRole("dialog", { name: "Create schedule" });
  await createSchedule.getByLabel("Name").fill(scheduleName);
  const scheduleTime = new Date(Date.now() + 5_000).toISOString().replace(/\.\d{3}Z$/, "");
  await createSchedule.getByLabel("Schedule expression").fill(`at(${scheduleTime})`);
  await createSchedule.getByLabel("Target ARN").fill(queueARN);
  await createSchedule
    .getByLabel("Execution role ARN")
    .fill("arn:aws:iam::123456789012:role/console-federation-role");
  await createSchedule.getByLabel("Target input").fill('"browser-to-scheduler"');
  await createSchedule.getByRole("button", { name: "Create schedule" }).click();
  await createSchedule.waitFor({ state: "detached" });
  await page.getByText(scheduleName, { exact: true }).waitFor({ state: "visible" });
  await page.waitForTimeout(7_000);
  await page.goto(`${origin}/ui/sqs`, { waitUntil: "domcontentloaded" });
  await page.getByRole("row", { name: new RegExp(queueName) }).getByRole("checkbox").click();
  await page.getByRole("button", { name: "Send and receive messages" }).click();
  const schedulerQueue = page.getByRole("dialog", { name: queueName });
  await schedulerQueue.getByRole("button", { name: "Poll for messages" }).click();
  await schedulerQueue.getByText(/browser-to-scheduler/).waitFor({ state: "visible" });
  await schedulerQueue.getByRole("button", { name: "Delete message" }).click();
  await schedulerQueue.getByRole("button", { name: "Close" }).last().click();

  await page.goto(`${origin}/ui/logs`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("logs-create-log-group").click();
  const createLogGroup = page.getByRole("dialog", { name: "Create log group" });
  await createLogGroup.getByTestId("logs-log-group-name-input").fill(logGroupName);
  await createLogGroup.getByTestId("logs-create-log-group-submit").click();
  await createLogGroup.waitFor({ state: "detached" });
  await page
    .getByTestId("log-groups-table")
    .getByText(logGroupName, { exact: true })
    .waitFor({ state: "visible" });
  await page.getByTestId("logs-storage-tier").click();
  const storageTier = page.getByRole("dialog", { name: "Account storage tier" });
  await storageTier.getByLabel("Storage tier").click();
  await page.getByRole("option", { name: "Intelligent-Tiering", exact: true }).click();
  await storageTier.getByTestId("logs-save-storage-tier").click();
  await storageTier.waitFor({ state: "detached" });
  await page.getByRole("row", { name: new RegExp(logGroupName) }).getByRole("checkbox").click();
  await page.getByTestId("logs-syslog-ingestion").click();
  const syslog = page.getByRole("dialog", { name: "Syslog ingestion" });
  await syslog.getByTestId("logs-create-syslog-configuration").click();
  await syslog.getByText("Service-managed endpoint", { exact: true }).waitFor({ state: "visible" });
  await syslog.getByRole("button", { name: "Delete" }).click();
  await syslog.getByText("No syslog configurations.", { exact: true }).waitFor({ state: "visible" });
  await syslog.getByRole("button", { name: "Close" }).last().click();

  await page.goto(`${origin}/ui/cloudwatch`, { waitUntil: "domcontentloaded" });
  await page.getByRole("button", { name: "Put metric data", exact: true }).click();
  const metric = page.getByRole("dialog", { name: "Put metric data" });
  await metric.getByRole("button", { name: "Put metric" }).click();
  await metric.getByText("Metric datum accepted.", { exact: true }).waitFor({ state: "visible" });
  await metric.getByRole("button", { name: "Close" }).last().click();
  await page.getByRole("button", { name: "Create alarm", exact: true }).click();
  const alarm = page.getByRole("dialog", { name: "Create metric alarm" });
  await alarm.getByLabel("Alarm name").fill(alarmName);
  await alarm.getByRole("button", { name: "Create alarm" }).click();
  await alarm.waitFor({ state: "detached" });
  await page
    .getByTestId("cloudwatch-alarms-table")
    .getByText(alarmName, { exact: true })
    .waitFor({ state: "visible" });
  await page.getByRole("button", { name: "Create dashboard", exact: true }).click();
  const dashboard = page.getByRole("dialog", { name: "Create dashboard" });
  await dashboard.getByLabel("Dashboard name").fill(dashboardName);
  await dashboard.getByRole("button", { name: "Create dashboard" }).click();
  await dashboard.waitFor({ state: "detached" });
  await page
    .getByTestId("cloudwatch-dashboards-table")
    .getByText(dashboardName, { exact: true })
    .waitFor({ state: "visible" });

  await page.goto(`${origin}/ui/cloudtrail`, { waitUntil: "domcontentloaded" });
  await page.getByRole("button", { name: "Create trail", exact: true }).click();
  const trail = page.getByRole("dialog", { name: "Create trail" });
  await trail.getByLabel("Trail name").fill(trailName);
  await trail.getByLabel("Amazon S3 bucket name").fill(`rps-cloudtrail-${suffix}`);
  await trail.getByRole("button", { name: "Create trail" }).click();
  await trail.waitFor({ state: "detached" });
  await page
    .getByTestId("cloudtrail-trails-table")
    .getByText(trailName, { exact: true })
    .waitFor({ state: "visible" });
  const eventTable = page.getByTestId("cloudtrail-events-table");
  await eventTable.getByRole("row").nth(1).getByRole("checkbox").click();
  await eventTable.getByRole("button", { name: "View event" }).click();
  const event = page.getByRole("dialog");
  await event.getByText("Event source", { exact: true }).waitFor({ state: "visible" });
  await event.getByRole("button", { name: "Close" }).last().click();
}

// assertManagedFirehoseAndPrivateCA proves the two source-service slices
// through the production browser data plane. Firehose assumes a provisioned
// IAM service role and delivers a real record to Amazon S3; AWS Private
// Certificate Authority performs the public root CSR → issue → import chain,
// then changes lifecycle state through the console.
async function assertManagedFirehoseAndPrivateCA(page, app) {
  const origin = new URL(app.launch).origin;
  const suffix = Date.now() % 1_000_000;
  const bucketName = `rps-firehose-${suffix}`;
  const streamName = `rps-firehose-${suffix}`;
  const commonName = `RPS Root CA ${suffix}`;

  await page.goto(`${origin}/ui/s3`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("s3-create-bucket").click();
  const bucket = page.getByRole("dialog", { name: "Create bucket" });
  await bucket.getByLabel("Bucket name").fill(bucketName);
  await bucket.getByTestId("s3-create-bucket-submit").click();
  await bucket.waitFor({ state: "detached" });
  await page.getByRole("link", { name: bucketName, exact: true }).waitFor({ state: "visible" });

  await page.goto(`${origin}/ui/firehose`, { waitUntil: "domcontentloaded" });
  await page.getByRole("button", { name: "Create delivery stream", exact: true }).click();
  const stream = page.getByRole("dialog", { name: "Create Firehose stream" });
  await stream.getByLabel("Delivery stream name").fill(streamName);
  await stream.getByLabel("Amazon S3 bucket ARN").fill(`arn:aws:s3:::${bucketName}`);
  await stream
    .getByLabel("IAM role ARN")
    .fill("arn:aws:iam::123456789012:role/console-firehose-role");
  await stream.getByLabel("S3 prefix").fill("browser/");
  await stream.getByTestId("firehose-create-submit").click();
  await stream.waitFor({ state: "detached" });
  const streamRow = page.getByRole("row", { name: new RegExp(streamName) });
  await streamRow.getByRole("checkbox").click();
  for (let index = 0; index < 2; index += 1) {
    await page.getByRole("button", { name: "Send test data" }).click();
    const record = page.getByRole("dialog", { name: `Send test record to ${streamName}` });
    await record
      .getByLabel("Record data")
      .fill(`browser-firehose-delivery-${index}\n${"x".repeat(600 * 1024)}`);
    await record.getByTestId("firehose-send-record-submit").click();
    await record.getByText(/Accepted record ID:/).waitFor({ state: "visible" });
    await record.getByRole("button", { name: "Close" }).last().click();
  }

  await page.goto(`${origin}/ui/s3/${bucketName}`, { waitUntil: "domcontentloaded" });
  await page.getByText(/browser\//).waitFor({ state: "visible" });

  await page.goto(`${origin}/ui/private-ca`, { waitUntil: "domcontentloaded" });
  await page.getByRole("button", { name: "Create CA", exact: true }).click();
  const authority = page.getByRole("dialog", { name: "Create root certificate authority" });
  await authority.getByLabel("Common name").fill(commonName);
  await authority.getByLabel("Organization").fill("Sockerless");
  await authority.getByTestId("private-ca-create-submit").click();
  await authority.waitFor({ state: "detached" });
  const authorityRow = page.getByRole("row", { name: new RegExp(commonName) });
  await authorityRow.getByText("ACTIVE", { exact: true }).waitFor({ state: "visible" });
  await authorityRow.getByRole("checkbox").click();
  await page.getByRole("button", { name: "Disable", exact: true }).click();
  await authorityRow.getByText("DISABLED", { exact: true }).waitFor({ state: "visible" });
  await page.getByRole("button", { name: "Enable", exact: true }).click();
  await authorityRow.getByText("ACTIVE", { exact: true }).waitFor({ state: "visible" });

  assert.equal(
    await page.getByTestId("private-ca-error").count() + await page.getByTestId("firehose-error").count(),
    0,
    `${app.name} Firehose or AWS Private Certificate Authority browser flow reported an API error`,
  );
}

// assertManagedAmplifyAndRDS proves the new operator workflows use the real
// federated cloud data plane. The independent official-client suite exercises
// the private Git clone/build and native database wire protocols; this browser
// flow pins their production console creation and connection experiences.
async function assertManagedAmplifyAndRDS(page, app) {
  const origin = new URL(app.launch).origin;
  const suffix = Date.now() % 1_000_000;
  const appName = `rps-amplify-${suffix}`;
  const databaseID = `rps-database-${suffix}`;

  await page.goto(`${origin}/ui/amplify`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("amplify-create-app").click();
  const amplify = page.getByRole("dialog", { name: "Create Amplify app" });
  await amplify.getByLabel("App name").fill(appName);
  await amplify.getByLabel("Repository URL").fill("https://github.com/example/private-site");
  await amplify.getByLabel("Repository access token").fill(`rps-repository-token-${suffix}`);
  await amplify.getByTestId("amplify-create-app-submit").click();
  await amplify.waitFor({ state: "detached" });
  const appRow = page.getByRole("row", { name: new RegExp(appName) });
  await appRow.getByText("TOKEN", { exact: true }).waitFor({ state: "visible" });

  await page.goto(`${origin}/ui/rds`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("rds-create-instance").click();
  const database = page.getByRole("dialog", { name: "Create database" });
  await database.getByLabel("DB instance identifier").fill(databaseID);
  await database.getByLabel("Master password").fill(`RpsDatabase-${suffix}!`);
  await database.getByTestId("rds-create-instance-submit").click();
  await database.waitFor({ state: "detached" });

  const databaseRow = page.getByRole("row", { name: new RegExp(databaseID) });
  await databaseRow.getByText("available", { exact: true }).waitFor({ state: "visible" });
  await databaseRow.getByRole("checkbox").click();
  await page.getByTestId("rds-connect-instance").click();
  const connection = page.getByRole("dialog", { name: `Connect to ${databaseID}` });
  await connection.getByText("IAM database authentication", { exact: true }).waitFor({ state: "visible" });
  await connection.getByText(/aws rds generate-db-auth-token/).waitFor({ state: "visible" });
  await connection.getByRole("button", { name: "Close" }).last().click();

  await page.getByTestId("rds-delete-instance").click();
  const deletion = page.getByRole("dialog", { name: `Delete ${databaseID}?` });
  await deletion.getByTestId("rds-delete-instance-confirm").click();
  await deletion.waitFor({ state: "detached" });
  await databaseRow.waitFor({ state: "detached" });

  assert.equal(
    await page.getByTestId("rds-instances-error").count(),
    0,
    `${app.name} Amazon RDS browser flow reported an API error`,
  );
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

// assertResourceDetailBladeRendersLiveData proves a resource detail blade
// renders live cloud data end to end, not just its shell: the signed-in operator
// opens the seeded Container Apps job, and the blade reads it over the federated
// Azure Resource Manager path and shows its real Essentials. A blade that
// couldn't reach the cloud would render its error alert instead.
async function assertResourceDetailBladeRendersLiveData(page, app) {
  const origin = new URL(app.launch).origin;
  const job = process.env.SOCKERLESS_RPS_AZURE_JOB;
  assert.ok(job, "SOCKERLESS_RPS_AZURE_JOB is required (the harness seeds the job)");
  await page.goto(`${origin}/ui/container-apps/${job}`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("ca-job-detail").waitFor({ state: "visible" });
  assert.equal(
    await page.getByTestId("ca-job-error").count(),
    0,
    `${app.name} Container Apps job detail blade failed to read the resource over the federated ARM path`,
  );
  // The blade's real Essentials must carry the resource's live values, read from
  // the seeded job's Azure Resource Manager record — the resource group parsed
  // from its id and the provisioning state the simulator assigned.
  await page.getByText("console-federation-rg").first().waitFor({ state: "visible" });
  assert.ok(
    await page.getByText("Succeeded").count() > 0,
    `${app.name} job detail Essentials did not render the live provisioning state`,
  );
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
