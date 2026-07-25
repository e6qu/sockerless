import { authorizedFetch, authorizedJSON, authorizedJSONPost } from "./portal/federation.js";

// The portal reads the real Azure Resource Manager and Azure Monitor APIs over
// federated, Microsoft Entra-issued credentials, rendering the true resource
// shapes rather than a hand-picked subset.

interface ArmList<T> {
  value?: T[];
}

interface ArmResource {
  id?: string;
  name?: string;
  location?: string;
  type?: string;
  kind?: string;
}

// API versions for each provider — the ones the real Azure Resource Manager
// serves these list operations at.
const API_VERSION = {
  subscriptions: "2022-12-01",
  // Microsoft.Subscription — the alias (subscription creation) API and the
  // cancel/rename/enable subscription actions.
  subscriptionAliases: "2021-10-01",
  jobs: "2024-03-01",
  sites: "2023-12-01",
  registries: "2023-07-01",
  storage: "2023-01-01",
  workspaces: "2022-10-01",
} as const;

// Azure resources live under subscriptions; the portal enumerates them the way
// the real portal does before listing a provider's resources across them.
export async function subscriptionIds(): Promise<string[]> {
  const list = await authorizedJSON<ArmList<{ subscriptionId?: string }>>(
    `/subscriptions?api-version=${API_VERSION.subscriptions}`,
  );
  return (list.value ?? []).map((s) => s.subscriptionId ?? "").filter(Boolean);
}

// --- Subscriptions blade: Azure Resource Manager subscriptions + the
// Microsoft.Subscription alias API (programmatic subscription creation) ---

export interface Subscription {
  id: string;
  subscriptionId: string;
  displayName: string;
  state: string;
}

interface ArmSubscription {
  id?: string;
  subscriptionId?: string;
  displayName?: string;
  state?: string;
}

function subscriptionOf(sub: ArmSubscription): Subscription {
  return {
    id: sub.id ?? "",
    subscriptionId: sub.subscriptionId ?? "",
    displayName: sub.displayName ?? "",
    state: sub.state ?? "",
  };
}

// armSend writes to an Azure Resource Manager path and surfaces ARM's own
// error message rather than masking it.
async function armSend(path: string, init: RequestInit = {}): Promise<Response> {
  const response = await authorizedFetch(path, "arm", init);
  if (!response.ok) {
    let detail = "";
    try {
      const body = (await response.json()) as { error?: { message?: string } };
      detail = body.error?.message ?? "";
    } catch {
      // A non-JSON error body keeps the HTTP status as the message.
    }
    throw new Error(`${path} returned HTTP ${response.status}${detail ? `: ${detail}` : ""}`);
  }
  return response;
}

export const fetchSubscriptions = async (): Promise<Subscription[]> => {
  const list = await authorizedJSON<ArmList<ArmSubscription>>(
    `/subscriptions?api-version=${API_VERSION.subscriptions}`,
  );
  return (list.value ?? []).map(subscriptionOf);
};

export const fetchSubscription = async (subscriptionId: string): Promise<Subscription> =>
  subscriptionOf(
    await authorizedJSON<ArmSubscription>(
      `/subscriptions/${subscriptionId}?api-version=${API_VERSION.subscriptions}`,
    ),
  );

export interface SubscriptionAlias {
  name: string;
  subscriptionId: string;
  provisioningState: string;
}

interface ArmSubscriptionAlias {
  name?: string;
  properties?: { subscriptionId?: string; provisioningState?: string };
}

function subscriptionAliasOf(alias: ArmSubscriptionAlias): SubscriptionAlias {
  return {
    name: alias.name ?? "",
    subscriptionId: alias.properties?.subscriptionId ?? "",
    provisioningState: alias.properties?.provisioningState ?? "",
  };
}

// createSubscriptionAlias drives the real subscription-creation flow the way
// the real portal (and `az account alias create`) does: a PUT on the
// Microsoft.Subscription alias with the display name, workload, and billing
// scope. The response reports provisioningState "Accepted" until the
// subscription materializes; getSubscriptionAlias is the poll.
export const createSubscriptionAlias = async (
  displayName: string,
  billingScope: string,
): Promise<SubscriptionAlias> => {
  const aliasName = crypto.randomUUID();
  const response = await armSend(
    `/providers/Microsoft.Subscription/aliases/${aliasName}?api-version=${API_VERSION.subscriptionAliases}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        properties: { displayName, workload: "Production", billingScope },
      }),
    },
  );
  return subscriptionAliasOf((await response.json()) as ArmSubscriptionAlias);
};

export const getSubscriptionAlias = async (aliasName: string): Promise<SubscriptionAlias> =>
  subscriptionAliasOf(
    await authorizedJSON<ArmSubscriptionAlias>(
      `/providers/Microsoft.Subscription/aliases/${aliasName}?api-version=${API_VERSION.subscriptionAliases}`,
    ),
  );

// cancelSubscription / enableSubscription are the real Microsoft.Subscription
// actions: a cancelled subscription becomes Disabled (it is never deleted —
// Azure has no subscription-delete API) and can be re-enabled.
export const cancelSubscription = async (subscriptionId: string): Promise<void> => {
  await armSend(
    `/subscriptions/${subscriptionId}/providers/Microsoft.Subscription/cancel?api-version=${API_VERSION.subscriptionAliases}`,
    { method: "POST" },
  );
};

export const enableSubscription = async (subscriptionId: string): Promise<void> => {
  await armSend(
    `/subscriptions/${subscriptionId}/providers/Microsoft.Subscription/enable?api-version=${API_VERSION.subscriptionAliases}`,
    { method: "POST" },
  );
};

async function listAcrossSubscriptions<T extends ArmResource>(provider: string, apiVersion: string): Promise<T[]> {
  const subs = await subscriptionIds();
  const pages = await Promise.all(
    subs.map((sub) =>
      authorizedJSON<ArmList<T>>(`/subscriptions/${sub}/providers/${provider}?api-version=${apiVersion}`),
    ),
  );
  return pages.flatMap((page) => page.value ?? []);
}

export interface ContainerAppJob {
  id: string;
  name: string;
  location: string;
  type: string;
}

export const fetchContainerAppJobs = async (): Promise<ContainerAppJob[]> => {
  const jobs = await listAcrossSubscriptions<ArmResource>("Microsoft.App/jobs", API_VERSION.jobs);
  return jobs.map((job) => ({
    id: job.id ?? "",
    name: job.name ?? "",
    location: job.location ?? "",
    type: job.type ?? "Microsoft.App/jobs",
  }));
};

export interface FunctionSite {
  id: string;
  name: string;
  location: string;
  kind: string;
}

export const fetchFunctionSites = async (): Promise<FunctionSite[]> => {
  const sites = await listAcrossSubscriptions<ArmResource>("Microsoft.Web/sites", API_VERSION.sites);
  return sites.map((site) => ({
    id: site.id ?? "",
    name: site.name ?? "",
    location: site.location ?? "",
    kind: site.kind ?? "",
  }));
};

export interface ACRRegistry {
  id: string;
  name: string;
  location: string;
}

export const fetchACRRegistries = async (): Promise<ACRRegistry[]> => {
  const registries = await listAcrossSubscriptions<ArmResource>(
    "Microsoft.ContainerRegistry/registries",
    API_VERSION.registries,
  );
  return registries.map((registry) => ({
    id: registry.id ?? "",
    name: registry.name ?? "",
    location: registry.location ?? "",
  }));
};

export interface StorageAccount {
  id: string;
  name: string;
  location: string;
  kind: string;
}

export const fetchStorageAccounts = async (): Promise<StorageAccount[]> => {
  const accounts = await listAcrossSubscriptions<ArmResource>(
    "Microsoft.Storage/storageAccounts",
    API_VERSION.storage,
  );
  return accounts.map((account) => ({
    id: account.id ?? "",
    name: account.name ?? "",
    location: account.location ?? "",
    kind: account.kind ?? "",
  }));
};

// resourceIdByName resolves a resource's real ARM resource ID from its short
// name. ARM's per-resource Get operations need the full
// subscription/resourceGroup-scoped path, which a route carrying only the
// resource's name doesn't have; this reads the same subscription-wide list
// the listing blade already reads (a real ARM List call) and returns the
// matching resource's own `id`, so the detail blade's subsequent Get reads
// the exact resource ARM would read for that ID.
async function resourceIdByName(provider: string, apiVersion: string, name: string): Promise<string> {
  const matches = await listAcrossSubscriptions<ArmResource>(provider, apiVersion);
  const match = matches.find((resource) => resource.name === name);
  if (!match?.id) {
    throw new Error(`${provider}/${name} was not found in any subscription this directory can reach.`);
  }
  return match.id;
}

// --- Container Apps job detail: containers + execution history ---
//
// The Container Apps blade this console lists (ContainerAppsPage) reads the
// real Microsoft.App/jobs resource — Azure Container Apps' run-to-completion
// model, the one sockerless deploys container tasks onto. The sim also
// implements the separate Microsoft.App/containerApps ("Apps") resource
// type, which is where `latestRevisionName` / `latestRevisionFqdn` live —
// but pass 1 never gave that resource type a list page or a menu entry, so
// nothing on this console reaches it today. This detail blade stays on the
// resource type the list page already shows: its own real Essentials
// (provisioning state, trigger type, replica timeout, environment), its own
// containers (from the job's template), and its own run history (the real
// executions list) — the Container Apps Jobs equivalent of a Cloud Run job's
// executions.

export interface ContainerAppJobContainer {
  name: string;
  image: string;
  command: string[];
  args: string[];
}

export interface ContainerAppJobDetail {
  id: string;
  name: string;
  location: string;
  provisioningState: string;
  environmentId: string;
  triggerType: string;
  replicaTimeout: number;
  replicaRetryLimit: number;
  containers: ContainerAppJobContainer[];
}

interface ArmContainerAppJob {
  id?: string;
  name?: string;
  location?: string;
  properties?: {
    provisioningState?: string;
    environmentId?: string;
    configuration?: { triggerType?: string; replicaTimeout?: number; replicaRetryLimit?: number };
    template?: {
      containers?: { name?: string; image?: string; command?: string[]; args?: string[] }[];
    };
  };
}

function containerAppJobOf(job: ArmContainerAppJob): ContainerAppJobDetail {
  return {
    id: job.id ?? "",
    name: job.name ?? "",
    location: job.location ?? "",
    provisioningState: job.properties?.provisioningState ?? "",
    environmentId: job.properties?.environmentId ?? "",
    triggerType: job.properties?.configuration?.triggerType ?? "",
    replicaTimeout: job.properties?.configuration?.replicaTimeout ?? 0,
    replicaRetryLimit: job.properties?.configuration?.replicaRetryLimit ?? 0,
    containers: (job.properties?.template?.containers ?? []).map((container) => ({
      name: container.name ?? "",
      image: container.image ?? "",
      command: container.command ?? [],
      args: container.args ?? [],
    })),
  };
}

export const fetchContainerAppJob = async (name: string): Promise<ContainerAppJobDetail> => {
  const id = await resourceIdByName("Microsoft.App/jobs", API_VERSION.jobs, name);
  return containerAppJobOf(await authorizedJSON<ArmContainerAppJob>(`${id}?api-version=${API_VERSION.jobs}`));
};

export interface ContainerAppJobExecution {
  name: string;
  status: string;
  startTime: string;
  endTime: string;
}

interface ArmContainerAppJobExecution {
  name?: string;
  properties?: { status?: string; startTime?: string; endTime?: string };
}

export const fetchContainerAppJobExecutions = async (jobId: string): Promise<ContainerAppJobExecution[]> => {
  const list = await authorizedJSON<ArmList<ArmContainerAppJobExecution>>(
    `${jobId}/executions?api-version=${API_VERSION.jobs}`,
  );
  return (list.value ?? []).map((execution) => ({
    name: execution.name ?? "",
    status: execution.properties?.status ?? "",
    startTime: execution.properties?.startTime ?? "",
    endTime: execution.properties?.endTime ?? "",
  }));
};

// startContainerAppJobExecution / stopContainerAppJobExecutions drive the
// real Container Apps Jobs start/stop actions — the same POSTs
// `az containerapp job start` / `az containerapp job stop-execution --all`
// issue.
export const startContainerAppJobExecution = async (jobId: string): Promise<void> => {
  await armSend(`${jobId}/start?api-version=${API_VERSION.jobs}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: "{}",
  });
};

export const stopContainerAppJobExecutions = async (jobId: string): Promise<void> => {
  await armSend(`${jobId}/stop?api-version=${API_VERSION.jobs}`, { method: "POST" });
};

// --- Function App detail: app settings + functions ---

export interface FunctionAppDetail {
  id: string;
  name: string;
  location: string;
  kind: string;
  state: string;
  defaultHostName: string;
  httpsOnly: boolean;
  enabled: boolean;
}

interface ArmSite {
  id?: string;
  name?: string;
  location?: string;
  kind?: string;
  properties?: { state?: string; defaultHostName?: string; httpsOnly?: boolean; enabled?: boolean };
}

function functionAppOf(site: ArmSite): FunctionAppDetail {
  return {
    id: site.id ?? "",
    name: site.name ?? "",
    location: site.location ?? "",
    kind: site.kind ?? "",
    state: site.properties?.state ?? "",
    defaultHostName: site.properties?.defaultHostName ?? "",
    httpsOnly: site.properties?.httpsOnly ?? false,
    enabled: site.properties?.enabled ?? false,
  };
}

export const fetchFunctionApp = async (name: string): Promise<FunctionAppDetail> => {
  const id = await resourceIdByName("Microsoft.Web/sites", API_VERSION.sites, name);
  return functionAppOf(await authorizedJSON<ArmSite>(`${id}?api-version=${API_VERSION.sites}`));
};

// fetchFunctionAppSettings reads app settings the real way: Microsoft.Web
// models `config/appsettings` as a write-only PUT resource and serves the
// current values only through the dedicated `/list` action POST (the same
// request `az functionapp config appsettings list` sends), because the
// values can carry secrets that stay out of GET URLs and proxy logs.
export const fetchFunctionAppSettings = async (siteId: string): Promise<Record<string, string>> => {
  const response = await armSend(`${siteId}/config/appsettings/list?api-version=${API_VERSION.sites}`, {
    method: "POST",
  });
  const body = (await response.json()) as { properties?: Record<string, string> };
  return body.properties ?? {};
};

export interface AzureFunctionSummary {
  name: string;
  language: string;
  isDisabled: boolean;
}

interface ArmFunctionEnvelope {
  name?: string;
  properties?: { language?: string; isDisabled?: boolean };
}

export const fetchFunctions = async (siteId: string): Promise<AzureFunctionSummary[]> => {
  const list = await authorizedJSON<ArmList<ArmFunctionEnvelope>>(`${siteId}/functions?api-version=${API_VERSION.sites}`);
  return (list.value ?? []).map((fn) => ({
    name: fn.name ?? "",
    language: fn.properties?.language ?? "",
    isDisabled: fn.properties?.isDisabled ?? false,
  }));
};

// --- Container registry (ACR) detail: repositories + tags ---

export interface ACRRegistryDetail {
  id: string;
  name: string;
  location: string;
  loginServer: string;
  skuName: string;
  skuTier: string;
  adminUserEnabled: boolean;
  provisioningState: string;
}

interface ArmRegistry {
  id?: string;
  name?: string;
  location?: string;
  sku?: { name?: string; tier?: string };
  properties?: { loginServer?: string; adminUserEnabled?: boolean; provisioningState?: string };
}

function acrRegistryOf(registry: ArmRegistry): ACRRegistryDetail {
  return {
    id: registry.id ?? "",
    name: registry.name ?? "",
    location: registry.location ?? "",
    loginServer: registry.properties?.loginServer ?? "",
    skuName: registry.sku?.name ?? "",
    skuTier: registry.sku?.tier ?? "",
    adminUserEnabled: registry.properties?.adminUserEnabled ?? false,
    provisioningState: registry.properties?.provisioningState ?? "",
  };
}

export const fetchACRRegistry = async (name: string): Promise<ACRRegistryDetail> => {
  const id = await resourceIdByName("Microsoft.ContainerRegistry/registries", API_VERSION.registries, name);
  return acrRegistryOf(await authorizedJSON<ArmRegistry>(`${id}?api-version=${API_VERSION.registries}`));
};

interface ACRAdminCredentials {
  username: string;
  password: string;
}

// mintACRAdminCredentials calls the real `listCredentials` ARM action — the
// same request `az acr credential show` sends — to obtain the registry's
// admin username/password. The ACR data plane (the registry's own login
// server, a distinct host from Azure Resource Manager) authenticates that
// pair as HTTP Basic, exactly the way `docker login <loginServer>` does with
// admin credentials.
async function mintACRAdminCredentials(registryId: string): Promise<ACRAdminCredentials> {
  const response = await armSend(`${registryId}/listCredentials?api-version=${API_VERSION.registries}`, {
    method: "POST",
  });
  const body = (await response.json()) as { username?: string; passwords?: { value?: string }[] };
  return { username: body.username ?? "", password: body.passwords?.[0]?.value ?? "" };
}

async function acrDataPlaneFetch(loginServer: string, path: string, credentials: ACRAdminCredentials): Promise<Response> {
  const url = `https://${loginServer}${path}`;
  const response = await fetch(url, {
    headers: { Authorization: `Basic ${btoa(`${credentials.username}:${credentials.password}`)}` },
  });
  if (!response.ok) {
    throw new Error(`${url} returned HTTP ${response.status}`);
  }
  return response;
}

// fetchACRRepositories reads the registry's own data-plane catalog — ACR's
// `/acr/v1/_catalog` convenience API, the same one `az acr repository list`
// calls — authenticated with the admin credentials minted above. Real ACR
// (and this simulator's registries.go) requires the admin user to be
// enabled for that credential pair to exist; a registry without it stays
// honestly unreadable from here rather than silently showing nothing.
export const fetchACRRepositories = async (registry: ACRRegistryDetail): Promise<string[]> => {
  if (!registry.adminUserEnabled) {
    throw new Error("Enable the admin user on this registry to browse its repositories from this console.");
  }
  const credentials = await mintACRAdminCredentials(registry.id);
  const response = await acrDataPlaneFetch(registry.loginServer, "/acr/v1/_catalog", credentials);
  const body = (await response.json()) as { repositories?: string[] };
  return body.repositories ?? [];
};

export interface ACRTag {
  name: string;
  digest: string;
}

export const fetchACRTags = async (registry: ACRRegistryDetail, repository: string): Promise<ACRTag[]> => {
  const credentials = await mintACRAdminCredentials(registry.id);
  const response = await acrDataPlaneFetch(
    registry.loginServer,
    `/acr/v1/${encodeURIComponent(repository)}/_tags`,
    credentials,
  );
  const body = (await response.json()) as { tags?: { name?: string; digest?: string }[] };
  return (body.tags ?? []).map((tag) => ({ name: tag.name ?? "", digest: tag.digest ?? "" }));
};

// --- Storage account detail: blob containers + blobs ---

export interface StorageAccountDetail {
  id: string;
  name: string;
  location: string;
  kind: string;
  skuName: string;
  provisioningState: string;
  accessTier: string;
  blobEndpoint: string;
}

interface ArmStorageAccount {
  id?: string;
  name?: string;
  location?: string;
  kind?: string;
  sku?: { name?: string };
  properties?: { provisioningState?: string; accessTier?: string; primaryEndpoints?: { blob?: string } };
}

function storageAccountOf(account: ArmStorageAccount): StorageAccountDetail {
  return {
    id: account.id ?? "",
    name: account.name ?? "",
    location: account.location ?? "",
    kind: account.kind ?? "",
    skuName: account.sku?.name ?? "",
    provisioningState: account.properties?.provisioningState ?? "",
    accessTier: account.properties?.accessTier ?? "",
    blobEndpoint: account.properties?.primaryEndpoints?.blob ?? "",
  };
}

export const fetchStorageAccount = async (name: string): Promise<StorageAccountDetail> => {
  const id = await resourceIdByName("Microsoft.Storage/storageAccounts", API_VERSION.storage, name);
  return storageAccountOf(await authorizedJSON<ArmStorageAccount>(`${id}?api-version=${API_VERSION.storage}`));
};

// mintAccountSas calls the real `ListAccountSas` ARM action — the same
// request the portal's own Storage Browser issues — to obtain a read-only
// account SAS scoped to the blob service, then uses it as the credential for
// the direct blob data-plane reads below.
async function mintAccountSas(accountId: string): Promise<string> {
  const signedExpiry = new Date(Date.now() + 60 * 60 * 1000).toISOString();
  const response = await armSend(`${accountId}/ListAccountSas?api-version=${API_VERSION.storage}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      signedServices: "b",
      signedResourceTypes: "sco",
      signedPermission: "rl",
      signedProtocol: "https",
      signedExpiry,
    }),
  });
  const body = (await response.json()) as { accountSasToken?: string };
  return body.accountSasToken ?? "";
}

function parseAzureStorageXML(xml: string): Document {
  return new DOMParser().parseFromString(xml, "application/xml");
}

function xmlText(scope: Element | null, tag: string): string {
  return scope?.querySelector(tag)?.textContent ?? "";
}

export interface BlobContainerSummary {
  name: string;
}

// parseContainerListXML reads the real Azure Storage `ListContainers`
// response shape (`EnumerationResults><Containers><Container><Name>`) — the
// same XML the .NET/Go/Python SDKs and `az storage container list` parse.
export function parseContainerListXML(xml: string): BlobContainerSummary[] {
  const doc = parseAzureStorageXML(xml);
  return Array.from(doc.querySelectorAll("Containers > Container")).map((node) => ({
    name: xmlText(node, "Name"),
  }));
}

export interface BlobSummary {
  name: string;
  contentLength: number;
  lastModified: string;
}

// parseBlobListXML reads the real `ListBlobs` response shape
// (`EnumerationResults><Blobs><Blob><Name>`/`<Properties>`).
export function parseBlobListXML(xml: string): BlobSummary[] {
  const doc = parseAzureStorageXML(xml);
  return Array.from(doc.querySelectorAll("Blobs > Blob")).map((node) => {
    const properties = node.querySelector("Properties");
    return {
      name: xmlText(node, "Name"),
      contentLength: Number(xmlText(properties, "Content-Length") || "0"),
      lastModified: xmlText(properties, "Last-Modified"),
    };
  });
}

export const fetchBlobContainers = async (account: StorageAccountDetail): Promise<BlobContainerSummary[]> => {
  if (!account.blobEndpoint) {
    throw new Error("This storage account has no blob endpoint to read.");
  }
  const sas = await mintAccountSas(account.id);
  const url = `${account.blobEndpoint}?comp=list&${sas}`;
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`${url} returned HTTP ${response.status}`);
  }
  return parseContainerListXML(await response.text());
};

export const fetchBlobs = async (account: StorageAccountDetail, container: string): Promise<BlobSummary[]> => {
  const sas = await mintAccountSas(account.id);
  const url = `${account.blobEndpoint}${encodeURIComponent(container)}?restype=container&comp=list&${sas}`;
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`${url} returned HTTP ${response.status}`);
  }
  return parseBlobListXML(await response.text());
};

// A Kusto result names its own columns; the portal reads the ones a container
// log query is expected to carry.
export interface MonitorLogRow {
  [key: string]: string;
}

interface WorkspaceResource extends ArmResource {
  properties?: { customerId?: string };
}

interface QueryTable {
  name?: string;
  columns?: { name?: string }[];
  rows?: unknown[][];
}

interface QueryResponse {
  tables?: QueryTable[];
}

// The portal reads logs the real way: it lists the Log Analytics workspaces,
// then runs a Kusto query against the workspace's own query endpoint (a distinct
// host and token audience from Azure Resource Manager) and shapes the tabular
// result into rows.
export const fetchMonitorLogs = async (): Promise<MonitorLogRow[]> => {
  const workspaces = await listAcrossSubscriptions<WorkspaceResource>(
    "Microsoft.OperationalInsights/workspaces",
    API_VERSION.workspaces,
  );
  const customerIds = workspaces.map((workspace) => workspace.properties?.customerId ?? "").filter(Boolean);

  const rows: MonitorLogRow[] = [];
  for (const customerId of customerIds) {
    const response = await authorizedJSONPost<QueryResponse>(
      `/v1/workspaces/${customerId}/query`,
      { query: "ContainerAppConsoleLogs_CL | take 100" },
      "logs",
    );
    for (const table of response.tables ?? []) {
      const names = (table.columns ?? []).map((column) => column.name ?? "");
      for (const row of table.rows ?? []) {
        const record: MonitorLogRow = {};
        names.forEach((name, index) => {
          record[name] = row[index] == null ? "" : String(row[index]);
        });
        rows.push(record);
      }
    }
    if (rows.length >= 100) break;
  }
  return rows.slice(0, 100);
};

// --- Microsoft Entra ID: App registrations (Microsoft Graph) ---
//
// The App registrations blade reads and writes the real Microsoft Graph
// application + service-principal surface (graph.microsoft.com's /v1.0 paths at
// the configured Graph coordinate) with a Graph-scoped federated token — the
// same requests the real Azure portal issues.

export interface ClientSecretMetadata {
  keyId: string;
  displayName: string;
  hint: string;
  startDateTime: string;
  endDateTime: string;
}

// MintedClientSecret is the addPassword response: the only place Microsoft
// Graph ever returns the secretText.
export interface MintedClientSecret extends ClientSecretMetadata {
  secretText: string;
}

export interface AppRegistration {
  id: string;
  appId: string;
  displayName: string;
  signInAudience: string;
  passwordCredentials: ClientSecretMetadata[];
}

interface GraphApplication {
  id?: string;
  appId?: string;
  displayName?: string;
  signInAudience?: string;
  passwordCredentials?: Partial<ClientSecretMetadata>[];
}

function appRegistrationOf(app: GraphApplication): AppRegistration {
  return {
    id: app.id ?? "",
    appId: app.appId ?? "",
    displayName: app.displayName ?? "",
    signInAudience: app.signInAudience ?? "",
    passwordCredentials: (app.passwordCredentials ?? []).map((credential) => ({
      keyId: credential.keyId ?? "",
      displayName: credential.displayName ?? "",
      hint: credential.hint ?? "",
      startDateTime: credential.startDateTime ?? "",
      endDateTime: credential.endDateTime ?? "",
    })),
  };
}

// graphSend writes to a Microsoft Graph path and surfaces Graph's own error
// message rather than masking it.
async function graphSend(path: string, init: RequestInit): Promise<Response> {
  const response = await authorizedFetch(path, "graph", init);
  if (!response.ok) {
    let detail = "";
    try {
      const body = (await response.json()) as { error?: { message?: string } };
      detail = body.error?.message ?? "";
    } catch {
      // A non-JSON error body keeps the HTTP status as the message.
    }
    throw new Error(`${path} returned HTTP ${response.status}${detail ? `: ${detail}` : ""}`);
  }
  return response;
}

export const fetchAppRegistrations = async (): Promise<AppRegistration[]> => {
  const list = await authorizedJSON<{ value?: GraphApplication[] }>("/v1.0/applications", "graph");
  return (list.value ?? []).map(appRegistrationOf);
};

export const fetchAppRegistration = async (objectId: string): Promise<AppRegistration> =>
  appRegistrationOf(await authorizedJSON<GraphApplication>(`/v1.0/applications/${objectId}`, "graph"));

// createAppRegistration provisions what the real portal's "New registration"
// provisions: the application object plus the service principal that
// materializes it in the tenant — the principal the client_credentials grant
// issues app-only tokens for.
export const createAppRegistration = async (displayName: string): Promise<AppRegistration> => {
  const created = await graphSend("/v1.0/applications", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ displayName, signInAudience: "AzureADMyOrg" }),
  });
  const app = appRegistrationOf((await created.json()) as GraphApplication);
  await graphSend("/v1.0/servicePrincipals", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ appId: app.appId }),
  });
  return app;
};

export const deleteAppRegistration = async (objectId: string): Promise<void> => {
  await graphSend(`/v1.0/applications/${objectId}`, { method: "DELETE" });
};

// addClientSecret mints a client secret on the application object — Microsoft
// Graph's addPassword, the call behind the Certificates & secrets blade. The
// response is the one time Graph returns the secretText.
export const addClientSecret = async (
  objectId: string,
  displayName: string,
  endDateTime: string,
): Promise<MintedClientSecret> => {
  const response = await graphSend(`/v1.0/applications/${objectId}/addPassword`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ passwordCredential: { displayName, endDateTime } }),
  });
  const credential = (await response.json()) as Partial<MintedClientSecret>;
  return {
    keyId: credential.keyId ?? "",
    displayName: credential.displayName ?? "",
    hint: credential.hint ?? "",
    startDateTime: credential.startDateTime ?? "",
    endDateTime: credential.endDateTime ?? "",
    secretText: credential.secretText ?? "",
  };
};

export const removeClientSecret = async (objectId: string, keyId: string): Promise<void> => {
  await graphSend(`/v1.0/applications/${objectId}/removePassword`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ keyId }),
  });
};
