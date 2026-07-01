import type {
  BleephubWorkflow,
  BleephubWorkflowFile,
  BleephubDispatchRequest,
  BleephubSession,
  BleephubRepo,
  RepoListFilters,
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
  GithubBranchProtection,
  GithubCommit,
  GithubWebhook,
  GithubSecret,
  GithubEnvironment,
  GithubRelease,
  GithubWorkflow,
  GithubWorkflowRun,
  GithubJob,
  GithubArtifact,
  GithubPendingDeployment,
  GithubCheckRun,
  GithubPublicKey,
  GithubVariable,
  GithubRunner,
  GithubContentFile,
  GithubOrgVisibility,
  GithubContentItem,
  BleephubUser,
  BleephubOrg,
  BleephubTeam,
  BleephubAuditEvent,
  BleephubGist,
  BleephubGistFile,
  GithubProjectClassic,
  GithubProjectColumn,
  GithubProjectCard,
  GithubSecretScanningAlert,
  GithubSecretScanningLocation,
  GithubSecretScanningResolution,
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

/** One page of a Link-paginated list plus parsed rel links. */
export interface Page<T> {
  items: T[];
  nextUrl: string | null;
  lastPage: number | null;
}

/** Extract the rel="last" page number from a GitHub-style Link header. */
export function parseLinkLast(link: string | null): number | null {
  if (!link) return null;
  for (const part of link.split(",")) {
    const m = part.match(/<[^>]*[?&]page=(\d+)[^>]*>\s*;\s*rel="last"/);
    if (m) return parseInt(m[1], 10);
  }
  return null;
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
  const link = res.headers.get("Link");
  return { items, nextUrl: parseLinkNext(link), lastPage: parseLinkLast(link) };
}

export const fetchRepoDetail = (owner: string, repo: string) =>
  ghFetch<BleephubRepo>(`/api/v3/repos/${owner}/${repo}`);

function buildRepoListURL(
  base: string,
  filters: RepoListFilters,
  perPage: number,
  pageUrl?: string,
): string {
  if (pageUrl) return pageUrl;
  const params = new URLSearchParams({ per_page: String(perPage) });
  if (filters.type) params.set("type", filters.type);
  if (filters.visibility) params.set("visibility", filters.visibility);
  if (filters.sort) params.set("sort", filters.sort);
  if (filters.direction) params.set("direction", filters.direction);
  return `${base}?${params}`;
}

/** First page of the authenticated user's repos; follow pages via Link header. */
export const fetchUserReposPage = (
  filters: RepoListFilters = {},
  pageUrl?: string,
): Promise<Page<BleephubRepo>> =>
  ghFetchPage<BleephubRepo>(
    buildRepoListURL("/api/v3/user/repos", filters, 30, pageUrl),
  );

/** First page of an organization's repos; follow pages via Link header. */
export const fetchOrgReposPage = (
  org: string,
  filters: RepoListFilters = {},
  pageUrl?: string,
): Promise<Page<BleephubRepo>> =>
  ghFetchPage<BleephubRepo>(
    buildRepoListURL(`/api/v3/orgs/${org}/repos`, filters, 30, pageUrl),
  );

export const createRepo = (payload: {
  name: string;
  description?: string;
  private?: boolean;
  visibility?: "public" | "private" | "internal";
  default_branch?: string;
  auto_init?: boolean;
  gitignore_template?: string;
  license_template?: string;
}): Promise<BleephubRepo> =>
  ghPostJSON("/api/v3/user/repos", payload);

export const createOrgRepo = (
  org: string,
  payload: {
    name: string;
    description?: string;
    private?: boolean;
    visibility?: "public" | "private" | "internal";
    default_branch?: string;
    auto_init?: boolean;
    gitignore_template?: string;
    license_template?: string;
  },
): Promise<BleephubRepo> => ghPostJSON(`/api/v3/orgs/${org}/repos`, payload);

export async function updateRepo(
  owner: string,
  repo: string,
  payload: Partial<BleephubRepo>,
): Promise<BleephubRepo> {
  const res = await fetch(`/api/v3/repos/${owner}/${repo}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    handleUnauthorized(res);
    const text = await res.text();
    throw new ApiError(res.status, `${res.status} ${res.statusText}: ${text || res.statusText}`);
  }
  return res.json() as Promise<BleephubRepo>;
}

export const fetchRepoContents = (
  owner: string,
  repo: string,
  path = "",
  ref?: string,
): Promise<GithubContentItem[]> => {
  const qs = ref ? `?ref=${encodeURIComponent(ref)}` : "";
  return ghFetch<GithubContentItem[]>(`/api/v3/repos/${owner}/${repo}/contents/${path}${qs}`);
};

export const fetchRepoReadme = (owner: string, repo: string, ref?: string): Promise<GithubContentFile> => {
  const qs = ref ? `?ref=${encodeURIComponent(ref)}` : "";
  return ghFetch<GithubContentFile>(`/api/v3/repos/${owner}/${repo}/readme${qs}`);
};

export const fetchRepoTopics = (owner: string, repo: string): Promise<{ names: string[] }> =>
  ghFetch<{ names: string[] }>(`/api/v3/repos/${owner}/${repo}/topics`);

export const updateRepoTopics = (owner: string, repo: string, names: string[]): Promise<{ names: string[] }> =>
  ghPutJSON(`/api/v3/repos/${owner}/${repo}/topics`, { names });

export const deleteRepoContent = (
  owner: string,
  repo: string,
  path: string,
  sha: string,
  message: string,
  branch?: string,
): Promise<unknown> => {
  const body: Record<string, string> = { message, sha };
  if (branch) body.branch = branch;
  return ghDeleteJSON(`/api/v3/repos/${owner}/${repo}/contents/${path}`, body);
};

async function ghPutJSON<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: "PUT",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    handleUnauthorized(res);
    const text = await res.text();
    throw new ApiError(res.status, `${res.status} ${res.statusText}: ${text || res.statusText}`);
  }
  return res.json() as Promise<T>;
}

async function ghDeleteJSON<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: "DELETE",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    handleUnauthorized(res);
    const text = await res.text();
    throw new ApiError(res.status, `${res.status} ${res.statusText}: ${text || res.statusText}`);
  }
  if (res.status === 204) {
    return undefined as T;
  }
  return res.json() as Promise<T>;
}

export const fetchGitignoreTemplates = () => ghFetch<string[]>("/api/v3/gitignore/templates");
export const fetchLicenseTemplates = () =>
  ghFetch<{ key: string; name: string; spdx_id: string }[]>("/api/v3/licenses");

async function ghPostJSON<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    handleUnauthorized(res);
    const text = await res.text();
    throw new ApiError(res.status, `${res.status} ${res.statusText}: ${text || res.statusText}`);
  }
  return res.json() as Promise<T>;
}

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

export const fetchBranchProtection = (owner: string, repo: string, branch: string) =>
  ghFetch<GithubBranchProtection>(`/api/v3/repos/${owner}/${repo}/branches/${encodeURIComponent(branch)}/protection`);

export const createBranchProtection = (
  owner: string,
  repo: string,
  branch: string,
  payload: Partial<GithubBranchProtection>,
) => ghPostJSON<GithubBranchProtection>(`/api/v3/repos/${owner}/${repo}/branches/${encodeURIComponent(branch)}/protection`, payload);

export const updateBranchProtection = (
  owner: string,
  repo: string,
  branch: string,
  payload: Partial<GithubBranchProtection>,
) => ghPutJSON<GithubBranchProtection>(`/api/v3/repos/${owner}/${repo}/branches/${encodeURIComponent(branch)}/protection`, payload);

export const deleteBranchProtection = (owner: string, repo: string, branch: string) =>
  ghDeleteJSON<void>(`/api/v3/repos/${owner}/${repo}/branches/${encodeURIComponent(branch)}/protection`, {});

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

// ─── GitHub Actions REST ────────────────────────────────────────────────

/**
 * One page of a GitHub envelope list ({total_count, <key>: [...]}) plus
 * the Link rel="next" URL. total_count is the full filtered count, not
 * the page size — list pages use it for "N workflow runs" headers.
 */
export interface EnvelopePage<T> {
  items: T[];
  totalCount: number;
  nextUrl: string | null;
}

async function ghFetchEnvelope<T>(url: string, key: string): Promise<EnvelopePage<T>> {
  const res = await fetch(url, { headers: authHeaders() });
  if (!res.ok) {
    handleUnauthorized(res);
    throw new ApiError(res.status, `${res.status} ${res.statusText}`);
  }
  const body = (await res.json()) as { total_count: number } & Record<string, T[]>;
  // No `?? []`: a missing array member is a contract break that must
  // surface as an error, not render as an empty list.
  const items = body[key];
  if (!Array.isArray(items)) {
    throw new Error(`malformed response: missing "${key}" array`);
  }
  return { items, totalCount: body.total_count, nextUrl: parseLinkNext(res.headers.get("Link")) };
}

/** Non-GET request that returns no JSON the caller renders. */
async function ghSend(method: string, path: string, body?: unknown): Promise<void> {
  const res = await fetch(path, {
    method,
    headers: body !== undefined
      ? { "Content-Type": "application/json", ...authHeaders() }
      : authHeaders(),
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    handleUnauthorized(res);
    const text = await res.text();
    throw new ApiError(res.status, `${method} ${res.status}: ${text || res.statusText}`);
  }
}

export const fetchActionsWorkflows = (owner: string, repo: string) =>
  ghFetchEnvelope<GithubWorkflow>(
    `/api/v3/repos/${owner}/${repo}/actions/workflows?per_page=100`,
    "workflows",
  );

/** Filters the runs-list endpoint supports server-side. */
export interface RunFilters {
  /** Numeric workflow-file id; scopes to .../workflows/{id}/runs. */
  workflowId?: number;
  status?: string;
  branch?: string;
  event?: string;
}

/** First page by filters; follow-up pages by the Link rel="next" URL. */
export function fetchWorkflowRunsPage(
  owner: string,
  repo: string,
  filters: RunFilters,
  pageUrl?: string,
): Promise<EnvelopePage<GithubWorkflowRun>> {
  if (pageUrl) return ghFetchEnvelope<GithubWorkflowRun>(pageUrl, "workflow_runs");
  const base = filters.workflowId
    ? `/api/v3/repos/${owner}/${repo}/actions/workflows/${filters.workflowId}/runs`
    : `/api/v3/repos/${owner}/${repo}/actions/runs`;
  const params = new URLSearchParams({ per_page: "30" });
  if (filters.status) params.set("status", filters.status);
  if (filters.branch) params.set("branch", filters.branch);
  if (filters.event) params.set("event", filters.event);
  return ghFetchEnvelope<GithubWorkflowRun>(`${base}?${params}`, "workflow_runs");
}

export const fetchWorkflowRun = (owner: string, repo: string, runId: number) =>
  ghFetch<GithubWorkflowRun>(`/api/v3/repos/${owner}/${repo}/actions/runs/${runId}`);

/**
 * Run shape for a specific attempt. 404s on servers that don't model
 * attempts — the caller treats that as "hide the attempt selector".
 */
export const fetchWorkflowRunAttempt = (
  owner: string,
  repo: string,
  runId: number,
  attempt: number,
) =>
  ghFetch<GithubWorkflowRun>(
    `/api/v3/repos/${owner}/${repo}/actions/runs/${runId}/attempts/${attempt}`,
  );

export const fetchRunJobs = (owner: string, repo: string, runId: number) =>
  ghFetchEnvelope<GithubJob>(
    `/api/v3/repos/${owner}/${repo}/actions/runs/${runId}/jobs?per_page=100`,
    "jobs",
  );

/** Job logs are text/plain, not JSON. */
export async function fetchJobLogs(owner: string, repo: string, jobId: number): Promise<string> {
  const res = await fetch(`/api/v3/repos/${owner}/${repo}/actions/jobs/${jobId}/logs`, {
    headers: authHeaders(),
  });
  if (!res.ok) {
    handleUnauthorized(res);
    throw new ApiError(res.status, `${res.status} ${res.statusText}`);
  }
  return res.text();
}

export const cancelRun = (owner: string, repo: string, runId: number) =>
  ghSend("POST", `/api/v3/repos/${owner}/${repo}/actions/runs/${runId}/cancel`);

export const rerunRun = (owner: string, repo: string, runId: number) =>
  ghSend("POST", `/api/v3/repos/${owner}/${repo}/actions/runs/${runId}/rerun`);

export const rerunFailedJobs = (owner: string, repo: string, runId: number) =>
  ghSend("POST", `/api/v3/repos/${owner}/${repo}/actions/runs/${runId}/rerun-failed-jobs`);

export const fetchRunArtifacts = (owner: string, repo: string, runId: number) =>
  ghFetchEnvelope<GithubArtifact>(
    `/api/v3/repos/${owner}/${repo}/actions/runs/${runId}/artifacts`,
    "artifacts",
  );

export const fetchPendingDeployments = (owner: string, repo: string, runId: number) =>
  ghFetch<GithubPendingDeployment[]>(
    `/api/v3/repos/${owner}/${repo}/actions/runs/${runId}/pending_deployments`,
  );

export const reviewPendingDeployments = (
  owner: string,
  repo: string,
  runId: number,
  body: { environment_ids: number[]; state: "approved" | "rejected"; comment: string },
) =>
  ghSend("POST", `/api/v3/repos/${owner}/${repo}/actions/runs/${runId}/pending_deployments`, body);

export const enableWorkflow = (owner: string, repo: string, workflowId: number) =>
  ghSend("PUT", `/api/v3/repos/${owner}/${repo}/actions/workflows/${workflowId}/enable`);

export const disableWorkflow = (owner: string, repo: string, workflowId: number) =>
  ghSend("PUT", `/api/v3/repos/${owner}/${repo}/actions/workflows/${workflowId}/disable`);

export const fetchFileContent = (owner: string, repo: string, path: string) =>
  ghFetch<GithubContentFile>(`/api/v3/repos/${owner}/${repo}/contents/${path}`);

export const fetchCheckRuns = (owner: string, repo: string, sha: string) =>
  ghFetchEnvelope<GithubCheckRun>(
    `/api/v3/repos/${owner}/${repo}/commits/${sha}/check-runs`,
    "check_runs",
  );

export const fetchActionsRunners = (owner: string, repo: string) =>
  ghFetchEnvelope<GithubRunner>(`/api/v3/repos/${owner}/${repo}/actions/runners`, "runners");

// ─── Secrets & variables (repo / environment / org scopes) ──────────────

/**
 * The three scopes GitHub stores Actions secrets + variables under. Each
 * maps to a URL prefix that `/secrets`, `/secrets/public-key`,
 * `/secrets/{name}`, `/variables` and `/variables/{name}` append to.
 */
export type SecretsScope =
  | { kind: "repo"; owner: string; repo: string }
  | { kind: "env"; owner: string; repo: string; env: string }
  | { kind: "org"; org: string };

function scopeBase(s: SecretsScope): string {
  switch (s.kind) {
    case "repo":
      return `/api/v3/repos/${s.owner}/${s.repo}/actions`;
    case "env":
      return `/api/v3/repos/${s.owner}/${s.repo}/environments/${encodeURIComponent(s.env)}`;
    case "org":
      return `/api/v3/orgs/${s.org}/actions`;
  }
}

export const fetchScopedSecrets = (scope: SecretsScope) =>
  ghFetchEnvelope<GithubSecret>(`${scopeBase(scope)}/secrets?per_page=100`, "secrets");

export const fetchScopedPublicKey = (scope: SecretsScope) =>
  ghFetch<GithubPublicKey>(`${scopeBase(scope)}/secrets/public-key`);

/** Body carries the sealed-box ciphertext only — plaintext never leaves the client. */
export const putScopedSecret = (
  scope: SecretsScope,
  name: string,
  body: { encrypted_value: string; key_id: string; visibility?: GithubOrgVisibility },
) => ghSend("PUT", `${scopeBase(scope)}/secrets/${encodeURIComponent(name)}`, body);

export const deleteScopedSecret = (scope: SecretsScope, name: string) =>
  ghSend("DELETE", `${scopeBase(scope)}/secrets/${encodeURIComponent(name)}`);

export const fetchScopedVariables = (scope: SecretsScope) =>
  ghFetchEnvelope<GithubVariable>(`${scopeBase(scope)}/variables?per_page=100`, "variables");

export const createScopedVariable = (
  scope: SecretsScope,
  body: { name: string; value: string; visibility?: GithubOrgVisibility },
) => ghSend("POST", `${scopeBase(scope)}/variables`, body);

export const updateScopedVariable = (
  scope: SecretsScope,
  name: string,
  body: { name: string; value: string },
) => ghSend("PATCH", `${scopeBase(scope)}/variables/${encodeURIComponent(name)}`, body);

export const deleteScopedVariable = (scope: SecretsScope, name: string) =>
  ghSend("DELETE", `${scopeBase(scope)}/variables/${encodeURIComponent(name)}`);

// ─── Internal admin endpoints ───────────────────────────────────────────

async function internalFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: { ...authHeaders(), ...(init?.headers || {}) },
  });
  if (!res.ok) {
    handleUnauthorized(res);
    throw new ApiError(res.status, `${res.status} ${res.statusText}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export const fetchUsers = () => internalFetch<BleephubUser[]>("/internal/users");

export const createUser = (payload: { login: string; password?: string; site_admin?: boolean }) =>
  internalFetch<BleephubUser>("/internal/users", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });

export const updateUser = (id: number, payload: Partial<BleephubUser>) =>
  internalFetch<BleephubUser>(`/internal/users/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });

export const deleteUser = (id: number) =>
  internalFetch<void>(`/internal/users/${id}`, { method: "DELETE" });

export const fetchOrgs = () => internalFetch<BleephubOrg[]>("/internal/orgs");

export const createOrg = (payload: { login: string; name?: string; description?: string; billing_email?: string }) =>
  internalFetch<BleephubOrg>("/internal/orgs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });

export const updateOrg = (id: number, payload: Partial<BleephubOrg>) =>
  internalFetch<BleephubOrg>(`/internal/orgs/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });

export const deleteOrg = (id: number) =>
  internalFetch<void>(`/internal/orgs/${id}`, { method: "DELETE" });

export const fetchTeams = () => internalFetch<BleephubTeam[]>("/internal/teams");

export const createTeam = (payload: { org: string; name: string; description?: string; privacy?: "secret" | "closed" }) =>
  internalFetch<BleephubTeam>("/internal/teams", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });

export const updateTeam = (id: number, payload: Partial<BleephubTeam>) =>
  internalFetch<BleephubTeam>(`/internal/teams/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });

export const deleteTeam = (id: number) =>
  internalFetch<void>(`/internal/teams/${id}`, { method: "DELETE" });

export const fetchAuditLog = (filters: {
  actor?: string;
  action?: string;
  entity_type?: string;
  since?: string;
  until?: string;
} = {}) => {
  const params = new URLSearchParams();
  Object.entries(filters).forEach(([k, v]) => {
    if (v) params.set(k, v);
  });
  const qs = params.toString();
  return internalFetch<BleephubAuditEvent[]>(`/internal/audit-log${qs ? `?${qs}` : ""}`);
};

export const fetchGists = () => internalFetch<BleephubGist[]>("/internal/gists");

export const fetchGist = (id: string) => internalFetch<BleephubGist>(`/internal/gists/${id}`);

export const createGist = (payload: {
  description: string;
  public: boolean;
  files: Record<string, { content: string }>;
}) =>
  internalFetch<BleephubGist>("/internal/gists", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });

export const updateGist = (
  id: string,
  payload: { description?: string; files?: Record<string, BleephubGistFile | null> },
) =>
  internalFetch<BleephubGist>(`/internal/gists/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });

export const deleteGist = (id: string) =>
  internalFetch<void>(`/internal/gists/${id}`, { method: "DELETE" });

// ─── GitHub Projects classic (v1) ───────────────────────────────────────

export const fetchProjectsClassic = (owner: string, repo: string) =>
  ghFetch<GithubProjectClassic[]>(`/api/v3/repos/${owner}/${repo}/projects`);

export const createProjectClassic = (
  owner: string,
  repo: string,
  payload: { name: string; body?: string; state?: "open" | "closed" },
) => ghPostJSON<GithubProjectClassic>(`/api/v3/repos/${owner}/${repo}/projects`, payload);

export const updateProjectClassic = (
  projectId: number,
  payload: Partial<{ name: string; body: string; state: "open" | "closed" }>,
) => ghPatchJSON<GithubProjectClassic>(`/api/v3/projects/${projectId}`, payload);

export const deleteProjectClassic = (projectId: number) =>
  ghDeleteJSON<void>(`/api/v3/projects/${projectId}`, {});

export const fetchProjectColumns = (projectId: number) =>
  ghFetch<GithubProjectColumn[]>(`/api/v3/projects/${projectId}/columns`);

export const createProjectColumn = (projectId: number, name: string) =>
  ghPostJSON<GithubProjectColumn>(`/api/v3/projects/${projectId}/columns`, { name });

export const updateProjectColumn = (columnId: number, name: string) =>
  ghPatchJSON<GithubProjectColumn>(`/api/v3/projects/columns/${columnId}`, { name });

export const deleteProjectColumn = (columnId: number) =>
  ghDeleteJSON<void>(`/api/v3/projects/columns/${columnId}`, {});

export const moveProjectColumn = (columnId: number, position: string) =>
  ghPostJSON<{ id: number; url: string }>(`/api/v3/projects/columns/${columnId}/moves`, { position });

export const fetchProjectCards = (columnId: number) =>
  ghFetch<GithubProjectCard[]>(`/api/v3/projects/columns/${columnId}/cards`);

export const createProjectCard = (
  columnId: number,
  payload: { note?: string; content_id?: number; content_type?: "Issue" },
) => ghPostJSON<GithubProjectCard>(`/api/v3/projects/columns/${columnId}/cards`, payload);

export const updateProjectCard = (cardId: number, note: string) =>
  ghPatchJSON<GithubProjectCard>(`/api/v3/projects/columns/cards/${cardId}`, { note });

export const deleteProjectCard = (cardId: number) =>
  ghDeleteJSON<void>(`/api/v3/projects/columns/cards/${cardId}`, {});

export const moveProjectCard = (
  cardId: number,
  payload: { position: string; column_id?: number },
) => ghPostJSON<{ id: number; url: string }>(`/api/v3/projects/columns/cards/${cardId}/moves`, payload);

// ─── Secret scanning ────────────────────────────────────────────────────

export interface SecretScanningFilters {
  state?: "open" | "resolved";
  secret_type?: string;
  resolution?: string;
}

export const fetchSecretScanningAlerts = (
  owner: string,
  repo: string,
  filters: SecretScanningFilters = {},
): Promise<GithubSecretScanningAlert[]> => {
  const params = new URLSearchParams();
  if (filters.state) params.set("state", filters.state);
  if (filters.secret_type) params.set("secret_type", filters.secret_type);
  if (filters.resolution) params.set("resolution", filters.resolution);
  const qs = params.toString();
  return ghFetch<GithubSecretScanningAlert[]>(`/api/v3/repos/${owner}/${repo}/secret-scanning/alerts${qs ? `?${qs}` : ""}`);
};

export const fetchSecretScanningAlert = (owner: string, repo: string, number: number) =>
  ghFetch<GithubSecretScanningAlert>(`/api/v3/repos/${owner}/${repo}/secret-scanning/alerts/${number}`);

export const fetchSecretScanningAlertLocations = (owner: string, repo: string, number: number) =>
  ghFetch<GithubSecretScanningLocation[]>(`/api/v3/repos/${owner}/${repo}/secret-scanning/alerts/${number}/locations`);

export const updateSecretScanningAlert = (
  owner: string,
  repo: string,
  number: number,
  body: { state: "open" | "resolved"; resolution?: GithubSecretScanningResolution; resolution_comment?: string },
) => ghPatchJSON<GithubSecretScanningAlert>(`/api/v3/repos/${owner}/${repo}/secret-scanning/alerts/${number}`, body);

async function ghPatchJSON<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: "PATCH",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    handleUnauthorized(res);
    const text = await res.text();
    throw new ApiError(res.status, `${res.status} ${res.statusText}: ${text || res.statusText}`);
  }
  return res.json() as Promise<T>;
}
