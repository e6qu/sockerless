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
