// Enum unions mirror the exact strings the server emits (bleephub Go:
// workflows.go / store_workflow_files.go). Empty result = still in flight.
// Keeping these as unions makes a typo'd comparison (e.g. "failed" vs the
// real "failure") a compile error rather than a silently-dead branch.
// Only values the server actually ASSIGNS belong here — a workflow is never
// "queued"/"skipped", a workflow file is never anything but "active".
// "waiting" = held on a reviewer-protected environment approval.
export type WorkflowStatus =
  | "running"
  | "completed"
  | "pending_concurrency"
  | "waiting";
export type JobStatus =
  | "pending"
  | "queued"
  | "running"
  | "completed"
  | "skipped"
  | "waiting";
export type JobResult = "success" | "failure" | "cancelled" | "skipped";
export type WorkflowResult = "" | JobResult;
export type WorkflowFileState = "active";
export type WorkflowFileSource = "submitted" | "discovered";

/**
 * Workflow represents a running multi-job workflow, as projected by the
 * management API's workflowView (handle_mgmt.go) — NOT the full Go
 * Workflow struct. Fields the view never emits (runNumber, ref, sha,
 * concurrencyGroup, …) are deliberately absent.
 */
export interface BleephubWorkflow {
  id: string;
  name: string;
  runId: number;
  jobs: Record<string, BleephubWorkflowJob>;
  status: WorkflowStatus;
  result: WorkflowResult;
  createdAt: string;
  eventName?: string;
  repoFullName?: string;
}

/** WorkflowJob represents a single job within a workflow. */
export interface BleephubWorkflowJob {
  key: string;
  jobId: string;
  displayName: string;
  needs?: string[];
  status: JobStatus;
  result: WorkflowResult;
  outputs?: Record<string, string>;
  matrix?: Record<string, unknown>;
  continueOnError?: boolean;
  /**
   * startedAt / completedAt are Go time.Time fields, always serialized —
   * a job that hasn't started/finished carries the zero-time sentinel
   * "0001-01-01T00:00:00Z" rather than omitting the field.
   */
  startedAt: string;
  completedAt: string;
  matrixGroup?: string;
}

/** Session represents a runner's active session. */
export interface BleephubSession {
  sessionId: string;
  ownerName: string;
  agent: BleephubAgent | null;
  pendingMessages: number;
}

/** Agent represents a registered runner agent. */
export interface BleephubAgent {
  id: number;
  name: string;
  version: string;
  enabled: boolean;
  status: string;
  osDescription: string;
  labels: BleephubLabel[];
  authorization?: BleephubAgentAuthorization;
  ephemeral?: boolean;
  maxParallelism?: number;
  provisioningState?: string;
  createdOn: string;
}

/** AgentAuthorization holds the agent's RSA public key and auth URL. */
export interface BleephubAgentAuthorization {
  authorizationUrl?: string;
  clientId?: string;
  publicKey?: { exponent: string; modulus: string };
}

/** Label is an agent label. */
export interface BleephubLabel {
  id: number;
  name: string;
  type: string;
}

/** Repo represents a GitHub repository. */
export interface BleephubRepo {
  id: number;
  name: string;
  full_name: string;
  description: string;
  default_branch: string;
  visibility: string;
  private: boolean;
  created_at: string;
  updated_at: string;
}

/** MetricsSnapshot is a point-in-time metrics report. */
export interface BleephubMetrics {
  workflow_submissions: number;
  job_dispatches: number;
  job_completions: Record<string, number>;
  active_workflows: number;
  active_sessions: number;
  uptime_seconds: number;
  goroutines: number;
  heap_alloc_mb: number;
}

/** Status response from /internal/status. */
export interface BleephubStatus {
  active_workflows: number;
  jobs_by_status: Record<string, number>;
  connected_runners: number;
  uptime_seconds: number;
}

/** Health response from /health. */
export interface BleephubHealth {
  status: string;
  service: string;
}

/** WorkflowFile is the file-level workflow YAML entity. */
export interface BleephubWorkflowFile {
  id: number;
  name: string;
  path: string;
  state: WorkflowFileState;
  repoFullName: string;
  source: WorkflowFileSource;
  createdAt: string;
  updatedAt: string;
}

/** Body for POST /api/v3/repos/{o}/{r}/actions/workflows/{id}/dispatches. */
export interface BleephubDispatchRequest {
  ref?: string;
  inputs?: Record<string, string>;
}

/** App row from /internal/apps (appView — id/slug/name/description/owner only). */
export interface BleephubApp {
  id: number;
  slug: string;
  name: string;
  description: string;
  ownerId: number;
  createdAt: string;
}

/** Installation row from /internal/installations. */
export interface BleephubInstallation {
  id: number;
  appId: number;
  appSlug: string;
  targetType: string;
  targetLogin: string;
  repositorySelection: string;
  createdAt: string;
  /** Always present on the wire; null when the installation is active. */
  suspendedAt: string | null;
}

/** OAuth App row from /internal/oauth-apps, distinct from GitHub App. */
export interface BleephubOAuthApp {
  clientId: string;
  name: string;
  description: string;
  url: string;
  callbackUrl: string;
  ownerId: number;
  createdAt: string;
}

// Wire shapes: the snake_case JSON the `/api/v3/bleephub/*` endpoints emit
// (server: oauthAppToJSON / appToJSON). Typing the raw response lets the
// snake→camel normalizers in api.ts drop their `as` casts, so a renamed or
// missing server field becomes a compile error at the mapping site.
export interface WireOAuthApp {
  client_id: string;
  name: string;
  description: string;
  url: string;
  callback_url: string;
  owner_id: number;
  created_at: string;
  updated_at: string;
}

/** The secret-bearing fields the GitHub-App create endpoint returns once. */
export interface WireAppCreated {
  client_id: string;
  pem: string;
  client_secret: string;
  webhook_secret: string;
}

/** Device-flow code from /internal/oauth/state. */
export interface BleephubDeviceCode {
  code: string;
  userCode: string;
  scopes: string;
  userId: number;
  expiresAt: string;
}

/** Authorization-code flow entry from /internal/oauth/state. */
export interface BleephubAuthCode {
  code: string;
  clientId: string;
  redirectUri: string;
  scopes: string;
  state: string;
  userId: number;
  createdAt: string;
  expiresAt: string;
}

export interface BleephubOAuthState {
  deviceCodes: BleephubDeviceCode[];
  authCodes: BleephubAuthCode[];
}

/** GitHub REST issue/PR state. */
export type GithubState = "open" | "closed";

/** GitHub Issue. */
export interface GithubIssue {
  id: number;
  number: number;
  title: string;
  body: string;
  state: GithubState;
  /** null when the authoring user no longer resolves (GitHub parity). */
  user: { login: string; avatar_url: string } | null;
  labels: { name: string; color: string }[];
  assignees: { login: string }[];
  comments: number;
  created_at: string;
  updated_at: string;
  closed_at: string | null;
}

/** GitHub Pull Request. */
export interface GithubPR {
  id: number;
  number: number;
  title: string;
  body: string;
  state: GithubState;
  draft: boolean;
  /** null when the authoring user no longer resolves (GitHub parity). */
  user: { login: string; avatar_url: string } | null;
  head: { ref: string; sha: string };
  base: { ref: string; sha: string };
  labels: { name: string; color: string }[];
  created_at: string;
  updated_at: string;
  merged_at: string | null;
  merged: boolean;
}

/** GitHub comment. */
export interface GithubComment {
  id: number;
  /** null when the authoring user no longer resolves (GitHub parity). */
  user: { login: string; avatar_url: string } | null;
  body: string;
  created_at: string;
  updated_at: string;
}

/** Git commit. */
export interface GithubCommit {
  sha: string;
  commit: {
    message: string;
    author: { name: string; email: string; date: string };
  };
}

/** Git branch. */
export interface GithubBranch {
  name: string;
  commit: { sha: string };
}

/** Storage backend info from /internal/storage */
export interface BleephubStorageInfo {
  persistence: string;
  dialect: string;
  git: string;
  git_details: Record<string, string>;
}

export interface GithubWebhook {
  id: number;
  name: string;
  active: boolean;
  events: string[];
  config: { url: string; content_type: string };
  created_at: string;
  updated_at: string;
  url: string;
  deliveries_url: string;
  last_response: { code: number | null; status: string; message: string | null };
}

export interface GithubSecret {
  name: string;
  created_at: string;
  updated_at: string;
}

export interface GithubEnvironment {
  name: string;
  node_id: string;
  url: string;
}

export interface GithubRelease {
  id: number;
  tag_name: string;
  name: string;
  body: string;
  draft: boolean;
  prerelease: boolean;
  created_at: string;
  /** null until the release is published (drafts). */
  published_at: string | null;
  html_url: string;
}
