async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json() as Promise<T>;
}

export interface ContainerAppJob {
  id: string;
  name: string;
  location: string;
  type: string;
}

export interface FunctionSite {
  id: string;
  name: string;
  location: string;
  kind: string;
}

export interface ACRRegistry {
  id: string;
  name: string;
  location: string;
}

export interface StorageAccount {
  id: string;
  name: string;
  location: string;
  kind: string;
}

export interface MonitorLogRow {
  [key: string]: unknown;
}

export const fetchContainerAppJobs = () => fetchJSON<ContainerAppJob[]>("/sim/v1/container-apps/jobs");
export const fetchFunctionSites = () => fetchJSON<FunctionSite[]>("/sim/v1/functions/sites");
export const fetchACRRegistries = () => fetchJSON<ACRRegistry[]>("/sim/v1/acr/registries");
export const fetchStorageAccounts = () => fetchJSON<StorageAccount[]>("/sim/v1/storage/accounts");
export const fetchMonitorLogs = () => fetchJSON<MonitorLogRow[]>("/sim/v1/monitor/logs");
