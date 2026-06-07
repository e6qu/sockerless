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
  BleephubOAuthApp,
  BleephubOAuthState,
  GithubIssue,
  GithubComment,
  GithubPR,
  GithubBranch,
  GithubCommit,
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
 * Returns the created App entity (with PEM private key + client_secret).
 */
export async function createApp(payload: {
  name: string;
  description?: string;
  permissions?: Record<string, string>;
  events?: string[];
}): Promise<BleephubApp & { pem: string; client_secret: string; webhook_secret: string }> {
  const res = await fetch("/api/v3/bleephub/apps", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: "Bearer bleephub-admin-token-00000000000000000000",
    },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`createApp ${res.status}: ${text || res.statusText}`);
  }
  return res.json();
}

export async function fetchOAuthApps(): Promise<BleephubOAuthApp[]> {
  const res = await fetch("/api/v3/bleephub/oauth-apps", {
    headers: { Authorization: "Bearer bleephub-admin-token-00000000000000000000" },
  });
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

export async function createOAuthApp(payload: {
  name: string;
  description?: string;
  url?: string;
  callback_url?: string;
}): Promise<BleephubOAuthApp & { client_secret: string }> {
  const res = await fetch("/api/v3/bleephub/oauth-apps", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: "Bearer bleephub-admin-token-00000000000000000000",
    },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`createOAuthApp ${res.status}: ${text || res.statusText}`);
  }
  return res.json();
}

export async function suspendInstallation(installationID: number, suspend: boolean): Promise<void> {
  const verb = suspend ? "suspend" : "unsuspend";
  const res = await fetch(`/api/v3/bleephub/installations/${installationID}/${verb}`, {
    method: "POST",
    headers: { Authorization: "Bearer bleephub-admin-token-00000000000000000000" },
  });
  if (!res.ok && res.status !== 409) {
    const text = await res.text();
    throw new Error(`${verb} ${res.status}: ${text || res.statusText}`);
  }
}

export async function deleteInstallation(installationID: number): Promise<void> {
  const res = await fetch(`/api/v3/bleephub/installations/${installationID}`, {
    method: "DELETE",
    headers: { Authorization: "Bearer bleephub-admin-token-00000000000000000000" },
  });
  if (!res.ok && res.status !== 404) {
    const text = await res.text();
    throw new Error(`delete ${res.status}: ${text || res.statusText}`);
  }
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

// --- GitHub REST API calls (same origin in production; proxied in dev) ---

const AUTH = "Bearer bleephub-admin-token-00000000000000000000";

async function ghFetch<T>(path: string): Promise<T> {
  const res = await fetch(path, { headers: { Authorization: AUTH } });
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json() as Promise<T>;
}

export const fetchRepoDetail = (owner: string, repo: string) =>
  ghFetch<BleephubRepo>(`/api/v3/repos/${owner}/${repo}`);

export const fetchRepoIssues = (owner: string, repo: string, state = "open") =>
  ghFetch<GithubIssue[]>(
    `/api/v3/repos/${owner}/${repo}/issues?state=${state}&per_page=50`
  );

export const fetchIssueDetail = (owner: string, repo: string, number: number) =>
  ghFetch<GithubIssue>(`/api/v3/repos/${owner}/${repo}/issues/${number}`);

export const fetchIssueComments = (owner: string, repo: string, number: number) =>
  ghFetch<GithubComment[]>(
    `/api/v3/repos/${owner}/${repo}/issues/${number}/comments`
  );

export const fetchRepoPRs = (owner: string, repo: string, state = "open") =>
  ghFetch<GithubPR[]>(
    `/api/v3/repos/${owner}/${repo}/pulls?state=${state}&per_page=50`
  );

export const fetchRepoBranches = (owner: string, repo: string) =>
  ghFetch<GithubBranch[]>(`/api/v3/repos/${owner}/${repo}/branches`);

export const fetchRepoCommits = (owner: string, repo: string) =>
  ghFetch<GithubCommit[]>(`/api/v3/repos/${owner}/${repo}/commits`);

export async function createIssue(
  owner: string,
  repo: string,
  payload: { title: string; body?: string },
): Promise<GithubIssue> {
  const res = await fetch(`/api/v3/repos/${owner}/${repo}/issues`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: AUTH },
    body: JSON.stringify(payload),
  });
  if (!res.ok) throw new Error(`createIssue ${res.status}`);
  return res.json();
}

export async function mergePR(
  owner: string,
  repo: string,
  number: number,
  mergeMethod = "merge",
): Promise<void> {
  const res = await fetch(`/api/v3/repos/${owner}/${repo}/pulls/${number}/merge`, {
    method: "PUT",
    headers: { "Content-Type": "application/json", Authorization: AUTH },
    body: JSON.stringify({ merge_method: mergeMethod }),
  });
  if (!res.ok && res.status !== 405) throw new Error(`merge ${res.status}`);
}
