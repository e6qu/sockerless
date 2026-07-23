import { authorizedJSON, authorizedJSONPost } from "./portal/federation.js";

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
async function subscriptionIds(): Promise<string[]> {
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
