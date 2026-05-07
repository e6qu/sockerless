import type {
  BleephubWorkflow,
  BleephubWorkflowFile,
  BleephubDispatchRequest,
  BleephubSession,
  BleephubRepo,
  BleephubMetrics,
  BleephubStatus,
  BleephubHealth,
  BleephubApp,
  BleephubInstallation,
  BleephubOAuthState,
} from "./types.js";

async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json() as Promise<T>;
}

export const fetchWorkflows = () =>
  fetchJSON<BleephubWorkflow[]>("/internal/workflows");

export const fetchWorkflowDetail = (id: string) =>
  fetchJSON<BleephubWorkflow>(`/internal/workflows/${id}`);

export const fetchWorkflowLogs = (id: string) =>
  fetchJSON<Record<string, string[]>>(`/internal/workflows/${id}/logs`);

export const fetchSessions = () =>
  fetchJSON<BleephubSession[]>("/internal/sessions");

export const fetchRepos = () =>
  fetchJSON<BleephubRepo[]>("/internal/repos");

export const fetchMetrics = () =>
  fetchJSON<BleephubMetrics>("/internal/metrics");

export const fetchStatus = () =>
  fetchJSON<BleephubStatus>("/internal/status");

export const fetchHealth = () =>
  fetchJSON<BleephubHealth>("/health");

export const fetchWorkflowFiles = () =>
  fetchJSON<BleephubWorkflowFile[]>("/internal/workflow_files");

export const fetchApps = () => fetchJSON<BleephubApp[]>("/internal/apps");
export const fetchInstallations = () =>
  fetchJSON<BleephubInstallation[]>("/internal/installations");
export const fetchOAuthState = () =>
  fetchJSON<BleephubOAuthState>("/internal/oauth/state");

/**
 * Create a new GitHub App via bleephub's management endpoint.
 * Returns the created App entity (with PEM private key).
 */
export async function createApp(payload: {
  name: string;
  description?: string;
}): Promise<BleephubApp & { pem: string }> {
  const res = await fetch("/api/v3/bleephub/apps", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`createApp ${res.status}: ${text || res.statusText}`);
  }
  return res.json();
}

export async function createInstallation(
  appId: number,
  payload: { targetType: string; targetId: number; targetLogin: string },
): Promise<BleephubInstallation> {
  const res = await fetch(`/api/v3/bleephub/apps/${appId}/installations`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`createInstallation ${res.status}: ${text || res.statusText}`);
  }
  return res.json();
}

/**
 * Trigger a workflow_dispatch run. The repo segment of the URL must
 * match the WorkflowFile's repoFullName; the workflow_id can be the
 * numeric ID or the YAML's filename.
 */
export async function dispatchWorkflow(
  repoFullName: string,
  workflowId: number | string,
  body: BleephubDispatchRequest = {},
): Promise<void> {
  const res = await fetch(
    `/api/v3/repos/${repoFullName}/actions/workflows/${workflowId}/dispatches`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    },
  );
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`dispatch ${res.status}: ${text || res.statusText}`);
  }
}
