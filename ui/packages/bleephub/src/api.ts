import type {
  BleephubWorkflow,
  BleephubWorkflowFile,
  BleephubDispatchRequest,
  BleephubSession,
  BleephubRepo,
  BleephubMetrics,
  BleephubStatus,
  BleephubHealth,
  BleephubStorageInfo,
  BleephubApp,
  BleephubInstallation,
  BleephubOAuthApp,
  BleephubOAuthState,
  WireOAuthApp,
  WireAppCreated,
  GithubIssue,
  GithubComment,
  GithubPR,
  GithubBranch,
  GithubCommit,
  GithubWebhook,
  GithubSecret,
  GithubEnvironment,
  GithubRelease,
} from "./types.js";

const TOKEN_KEY = "bleephub_token";

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
}

export function isLoggedIn(): boolean {
  return !!getToken();
}

export function authHeaders(): Record<string, string> {
  const token = getToken();
  if (!token) return {};
  return { Authorization: `Bearer ${token}` };
}

// A 401 means the stored token is no longer valid (e.g. the server
// restarted with in-memory persistence) — clear it and send the user
// back to the login form instead of spinning forever.
function handleUnauthorized(res: Response): void {
  if (res.status === 401) {
    clearToken();
    window.location.href = "/ui/login";
  }
}

/** Error carrying the HTTP status so callers can branch on 404 vs failure. */
export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export function isNotFound(err: unknown): boolean {
  return err instanceof ApiError && err.status === 404;
}

async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url, { headers: authHeaders() });
  if (!res.ok) {
    handleUnauthorized(res);
    throw new ApiError(res.status, `${res.status} ${res.statusText}`);
  }
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

export const fetchStorageInfo = () =>
  fetchJSON<BleephubStorageInfo>("/internal/storage");

// Verify against an /internal endpoint, not /api/v3/user: the dashboard's
// data all lives under /internal/*, which only accepts PATs (incl. the
// admin token). /api/v3/user also accepts gho_/ghu_/ghs_ tokens, which
// would let login "succeed" and then bounce straight back on the first
// dashboard fetch. No handleUnauthorized here — a 401 during login is the
// verdict, not a stale-session redirect.
export async function verifyToken(token: string): Promise<boolean> {
  const res = await fetch("/internal/status", {
    headers: { Authorization: `Bearer ${token}` },
  });
  return res.ok;
}

// The /internal/* create + oauth-apps management endpoints return GitHub's
// snake_case wire shape (client_id, callback_url, created_at). The UI's
// types are camelCase, so normalize at this boundary. Fields are mapped
// 1:1 from the server contract — no defaults, so a contract break shows
// as undefined rather than a plausible-looking blank.
function normalizeOAuthApp(raw: WireOAuthApp): BleephubOAuthApp {
  return {
    clientId: raw.client_id,
    name: raw.name,
    description: raw.description,
    url: raw.url,
    callbackUrl: raw.callback_url,
    ownerId: raw.owner_id,
    createdAt: raw.created_at,
  };
}

export async function createApp(payload: {
  name: string;
  description?: string;
  permissions?: Record<string, string>;
  events?: string[];
}): Promise<{ clientId: string; pem: string; client_secret: string; webhook_secret: string }> {
  const res = await fetch("/internal/apps", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...authHeaders(),
    },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`createApp ${res.status}: ${text || res.statusText}`);
  }
  // appToJSON returns the GitHub snake_case app shape plus the once-shown
  // secrets; the create dialog only needs the client id + secrets, surfaced
  // here as the camelCase clientId it reads.
  const raw = (await res.json()) as WireAppCreated;
  return {
    clientId: raw.client_id,
    pem: raw.pem,
    client_secret: raw.client_secret,
    webhook_secret: raw.webhook_secret,
  };
}

export async function fetchOAuthApps(): Promise<BleephubOAuthApp[]> {
  const res = await fetch("/internal/oauth-apps", {
    headers: authHeaders(),
  });
  if (!res.ok) {
    handleUnauthorized(res);
    throw new Error(`${res.status} ${res.statusText}`);
  }
  const raw = (await res.json()) as WireOAuthApp[];
  return raw.map(normalizeOAuthApp);
}

export async function createOAuthApp(payload: {
  name: string;
  description?: string;
  url?: string;
  callback_url?: string;
}): Promise<BleephubOAuthApp & { client_secret: string }> {
  const res = await fetch("/internal/oauth-apps", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...authHeaders(),
    },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`createOAuthApp ${res.status}: ${text || res.statusText}`);
  }
  const raw = (await res.json()) as WireOAuthApp & { client_secret: string };
  return {
    ...normalizeOAuthApp(raw),
    client_secret: raw.client_secret,
  };
}

export async function suspendInstallation(installationID: number, suspend: boolean): Promise<void> {
  const verb = suspend ? "suspend" : "unsuspend";
  const res = await fetch(`/internal/installations/${installationID}/${verb}`, {
    method: "POST",
    headers: authHeaders(),
  });
  if (!res.ok && res.status !== 409) {
    const text = await res.text();
    throw new Error(`${verb} ${res.status}: ${text || res.statusText}`);
  }
}

export async function deleteInstallation(installationID: number): Promise<void> {
  const res = await fetch(`/internal/installations/${installationID}`, {
    method: "DELETE",
    headers: authHeaders(),
  });
  if (!res.ok && res.status !== 404) {
    const text = await res.text();
    throw new Error(`delete ${res.status}: ${text || res.statusText}`);
  }
}

export async function dispatchWorkflow(
  repoFullName: string,
  workflowId: number | string,
  body: BleephubDispatchRequest = {},
): Promise<void> {
  const res = await fetch(
    `/api/v3/repos/${repoFullName}/actions/workflows/${workflowId}/dispatches`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json", ...authHeaders() },
      body: JSON.stringify(body),
    },
  );
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`dispatch ${res.status}: ${text || res.statusText}`);
  }
}

async function ghFetch<T>(path: string): Promise<T> {
  const res = await fetch(path, { headers: authHeaders() });
  if (!res.ok) {
    handleUnauthorized(res);
    throw new ApiError(res.status, `${res.status} ${res.statusText}`);
  }
  return res.json() as Promise<T>;
}

/** One page of a Link-paginated list plus the rel="next" URL (null = last page). */
export interface Page<T> {
  items: T[];
  nextUrl: string | null;
}

/** Extract the rel="next" target from a GitHub-style Link header. */
export function parseLinkNext(link: string | null): string | null {
  if (!link) return null;
  for (const part of link.split(",")) {
    const m = part.match(/<([^>]+)>\s*;\s*rel="next"/);
    if (m) return m[1];
  }
  return null;
}

// The server paginates list endpoints (per_page max 100) and advertises the
// follow-up page via the Link header — honor it instead of silently showing
// only the first 50 items.
async function ghFetchPage<T>(url: string): Promise<Page<T>> {
  const res = await fetch(url, { headers: authHeaders() });
  if (!res.ok) {
    handleUnauthorized(res);
    throw new ApiError(res.status, `${res.status} ${res.statusText}`);
  }
  const items = (await res.json()) as T[];
  return { items, nextUrl: parseLinkNext(res.headers.get("Link")) };
}

export const fetchRepoDetail = (owner: string, repo: string) =>
  ghFetch<BleephubRepo>(`/api/v3/repos/${owner}/${repo}`);

/** First page by (owner, repo, state); follow-up pages by the Link rel="next" URL. */
export const fetchRepoIssuesPage = (
  owner: string,
  repo: string,
  state = "open",
  pageUrl?: string,
) =>
  ghFetchPage<GithubIssue>(
    pageUrl ?? `/api/v3/repos/${owner}/${repo}/issues?state=${state}&per_page=50`
  );

export const fetchIssueDetail = (owner: string, repo: string, number: number) =>
  ghFetch<GithubIssue>(`/api/v3/repos/${owner}/${repo}/issues/${number}`);

export const fetchIssueComments = (owner: string, repo: string, number: number) =>
  ghFetch<GithubComment[]>(
    `/api/v3/repos/${owner}/${repo}/issues/${number}/comments`
  );

/** First page by (owner, repo, state); follow-up pages by the Link rel="next" URL. */
export const fetchRepoPRsPage = (
  owner: string,
  repo: string,
  state = "open",
  pageUrl?: string,
) =>
  ghFetchPage<GithubPR>(
    pageUrl ?? `/api/v3/repos/${owner}/${repo}/pulls?state=${state}&per_page=50`
  );

export const fetchPRDetail = (owner: string, repo: string, number: number) =>
  ghFetch<GithubPR>(`/api/v3/repos/${owner}/${repo}/pulls/${number}`);

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
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`createIssue ${res.status}: ${text || res.statusText}`);
  }
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
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ merge_method: mergeMethod }),
  });
  if (!res.ok) {
    handleUnauthorized(res);
    // 405 = "not mergeable" (already merged/closed) — a real failure the
    // caller must see, not a success path.
    const text = await res.text();
    throw new Error(`merge ${res.status}: ${text || res.statusText}`);
  }
}

export const fetchWebhooks = (owner: string, repo: string) =>
  ghFetch<GithubWebhook[]>(`/api/v3/repos/${owner}/${repo}/hooks`);

// Secrets + environments come back in GitHub's list envelope
// ({secrets:[…], total_count}) — unwrap to the array the UI renders.
// No `?? []`: if the server ever stops sending the array, the missing
// field should surface as an error, not a silent "none configured".
export const fetchSecrets = (owner: string, repo: string) =>
  ghFetch<{ secrets: GithubSecret[] }>(
    `/api/v3/repos/${owner}/${repo}/actions/secrets`
  ).then((r) => r.secrets);

export const fetchEnvironments = (owner: string, repo: string) =>
  ghFetch<{ environments: GithubEnvironment[] }>(
    `/api/v3/repos/${owner}/${repo}/environments`
  ).then((r) => r.environments);

export const fetchReleases = (owner: string, repo: string) =>
  ghFetch<GithubRelease[]>(`/api/v3/repos/${owner}/${repo}/releases`);
