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

/** Filters the repo list endpoints support server-side. */
export interface RepoListFilters {
  type?: string;
  visibility?: "public" | "private" | "internal";
  sort?: "created" | "updated" | "pushed" | "full_name";
  direction?: "asc" | "desc";
}

/** Repo represents a GitHub repository. */
export interface BleephubRepo {
  id: number;
  node_id: string;
  name: string;
  full_name: string;
  description: string;
  homepage: string | null;
  default_branch: string;
  visibility: string;
  private: boolean;
  created_at: string;
  updated_at: string;
  pushed_at: string | null;
  size: number;
  owner: { login: string; type: string; avatar_url?: string };
  organization?: { login: string; type: string; avatar_url?: string };
  license: { key: string; name: string; spdx_id: string; url: string; node_id: string } | null;
  has_issues: boolean;
  has_projects: boolean;
  has_wiki: boolean;
  has_pull_requests: boolean;
  is_template: boolean;
  archived: boolean;
  web_commit_signoff_required: boolean;
  allow_squash_merge: boolean;
  allow_merge_commit: boolean;
  allow_rebase_merge: boolean;
  allow_auto_merge: boolean;
  allow_update_branch: boolean;
  delete_branch_on_merge: boolean;
  use_squash_pr_title_as_default: boolean;
  squash_merge_commit_title: string;
  squash_merge_commit_message: string;
  merge_commit_title: string;
  merge_commit_message: string;
  pull_request_creation_policy: string;
  topics?: string[];
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
  /** Only present on the single-PR detail response, not list items. */
  mergeable_state?: "clean" | "dirty" | "blocked" | "unstable" | "unknown";
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

export interface GithubStatusCheck {
  context: string;
  app_id: number | null;
}

export interface GithubBranchProtectionStatusChecks {
  strict?: boolean;
  enforcement_level: string;
  contexts: string[];
  checks: GithubStatusCheck[];
  include_admins?: boolean;
}

export interface GithubBranchProtectionReviewDismissalRestrictions {
  users: GithubActor[];
  teams: GithubTeamRef[];
  apps?: GithubActor[];
  url?: string;
  users_url?: string;
  teams_url?: string;
}

export interface GithubActor {
  login: string;
  id: number;
  node_id: string;
  avatar_url: string;
  html_url: string;
  type: string;
  site_admin: boolean;
}

export interface GithubTeamRef {
  id: number;
  node_id: string;
  url: string;
  html_url: string;
  name: string;
  slug: string;
  description: string | null;
  privacy: string;
  permission: string;
}

export interface GithubBranchProtectionReviews {
  url?: string;
  dismissal_restrictions?: GithubBranchProtectionReviewDismissalRestrictions;
  dismiss_stale_reviews: boolean;
  require_code_owner_reviews: boolean;
  required_approving_review_count: number;
  bypass_pull_request_allowances?: GithubBranchProtectionBypassAllowances;
  require_last_push_approval?: boolean;
  required_review_thread_resolution?: boolean;
}

export interface GithubBranchProtectionBypassAllowances {
  users: GithubActor[];
  teams: GithubTeamRef[];
  apps?: GithubActor[];
}

export interface GithubBranchProtectionRestrictions {
  url: string;
  users_url: string;
  teams_url: string;
  apps_url?: string;
  users: GithubActor[];
  teams: GithubTeamRef[];
  apps?: GithubActor[];
}

export interface GithubProtectionToggle {
  enabled: boolean;
  url?: string;
  html_url?: string;
}

/** Branch protection configuration from /api/v3/repos/{o}/{r}/branches/{b}/protection */
export interface GithubBranchProtection {
  url: string;
  html_url: string;
  required_status_checks: GithubBranchProtectionStatusChecks | null;
  required_pull_request_reviews: GithubBranchProtectionReviews | null;
  restrictions: GithubBranchProtectionRestrictions | null;
  enforce_admins: { url?: string; enabled: boolean } | null;
  allow_force_pushes: GithubProtectionToggle;
  allow_deletions: GithubProtectionToggle;
  required_conversation_resolution?: GithubProtectionToggle;
  required_linear_history?: GithubProtectionToggle;
  required_signatures?: GithubProtectionToggle;
  lock_branch?: GithubProtectionToggle;
  block_creations?: GithubProtectionToggle;
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
  /** Org-scope secrets only (all | private | selected). */
  visibility?: GithubOrgVisibility;
}

export interface GithubEnvironment {
  id: number;
  name: string;
  node_id: string;
  url: string;
}

// ─── GitHub Actions REST shapes (/api/v3/repos/{o}/{r}/actions/*) ───────

/** GitHub workflow-run status. */
export type GHRunStatus = "queued" | "in_progress" | "completed" | "waiting";
/** GitHub run/job/step conclusion (null while in flight). */
export type GHConclusion =
  | "success"
  | "failure"
  | "cancelled"
  | "skipped"
  | "neutral"
  | "timed_out"
  | "action_required";

/** Workflow run — GET .../actions/runs (items) + .../actions/runs/{id}. */
export interface GithubWorkflowRun {
  id: number;
  name: string;
  run_number: number;
  run_attempt: number;
  event: string;
  status: GHRunStatus;
  conclusion: GHConclusion | null;
  head_branch: string;
  head_sha: string;
  path: string;
  workflow_id: number;
  created_at: string;
  updated_at: string;
  /** null when the server can't attribute the run to a user. */
  actor: { login: string } | null;
}

/** Workflow file — GET .../actions/workflows (items). */
export interface GithubWorkflow {
  id: number;
  name: string;
  path: string;
  state: "active" | "disabled_manually" | "disabled_inactivity";
  badge_url: string;
}

/** Per-step entry inside a job. */
export interface GithubJobStep {
  name: string;
  status: GHRunStatus;
  conclusion: GHConclusion | null;
  number: number;
  started_at: string | null;
  completed_at: string | null;
}

/** Job — GET .../actions/runs/{run_id}/jobs (items). */
export interface GithubJob {
  id: number;
  run_id: number;
  name: string;
  status: GHRunStatus;
  conclusion: GHConclusion | null;
  started_at: string | null;
  completed_at: string | null;
  steps: GithubJobStep[];
  labels: string[];
  run_attempt: number;
}

/** Artifact — GET .../actions/runs/{run_id}/artifacts (items). */
export interface GithubArtifact {
  id: number;
  name: string;
  size_in_bytes: number;
  expired: boolean;
  created_at: string;
}

/** Pending deployment — GET .../actions/runs/{run_id}/pending_deployments. */
export interface GithubPendingDeployment {
  environment: { id: number; name: string };
  wait_timer: number;
  wait_timer_started_at: string | null;
  current_user_can_approve: boolean;
  reviewers: { type: string; reviewer?: { login?: string; name?: string } }[];
}

/** Check run — GET .../commits/{sha}/check-runs (items). */
export interface GithubCheckRun {
  id: number;
  name: string;
  status: GHRunStatus;
  conclusion: GHConclusion | null;
  started_at: string | null;
  completed_at: string | null;
  details_url: string;
  html_url?: string;
  app: { id: number } | null;
}

/** Actions secrets public key — GET {scope}/secrets/public-key. */
export interface GithubPublicKey {
  key_id: string;
  /** base64-encoded 32-byte X25519 public key for sealed-box encryption. */
  key: string;
}

export type GithubOrgVisibility = "all" | "private" | "selected";

/** Actions variable — GET {scope}/variables (items). */
export interface GithubVariable {
  name: string;
  value: string;
  created_at: string;
  updated_at: string;
  /** Org-scope variables only. */
  visibility?: GithubOrgVisibility;
}

/** Self-hosted runner — GET .../actions/runners (items). */
export interface GithubRunner {
  id: number;
  name: string;
  os: string;
  status: "online" | "offline";
  busy: boolean;
  labels: { id: number; name: string; type: string }[];
}

/** Content-file response — GET .../contents/{path} (file variant). */
export interface GithubContentFile {
  name: string;
  path: string;
  sha: string;
  type: string;
  encoding: string;
  content: string;
}

/** Content directory entry — GET .../contents/{path} (dir variant). */
export interface GithubContentItem {
  name: string;
  path: string;
  sha: string;
  type: "file" | "dir";
  size?: number;
}

/** `on.workflow_dispatch.inputs.<name>` entry parsed from workflow YAML. */
export interface WorkflowDispatchInput {
  description?: string;
  required?: boolean;
  default?: string | boolean;
  type?: "string" | "choice" | "boolean" | "environment" | "number";
  options?: string[];
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

export type GithubMigrationState = "pending" | "exporting" | "exported" | "failed";

/** Request body for POST /user/migrations and /orgs/{org}/migrations. */
export interface GithubMigrationStartPayload {
  repositories: string[];
  lock_repositories?: boolean;
  exclude_metadata?: boolean;
  exclude_git_data?: boolean;
  exclude_attachments?: boolean;
  exclude_releases?: boolean;
  exclude_owner_projects?: boolean;
  org_metadata_only?: boolean;
}

/** GitHub migration export object (Migrations REST API). */
export interface GithubMigration {
  id: number;
  node_id: string;
  guid: string;
  state: GithubMigrationState;
  repositories: BleephubRepo[];
  lock_repositories: boolean;
  exclude_metadata: boolean;
  exclude_git_data: boolean;
  exclude_attachments: boolean;
  exclude_releases: boolean;
  exclude_owner_projects: boolean;
  org_metadata_only: boolean;
  url: string;
  html_url: string;
  archive_url: string;
  created_at: string;
  updated_at: string;
  exported_at: string;
}

export interface BleephubUser {
  id: number;
  login: string;
  type: "User" | "Bot" | "Organization";
  site_admin: boolean;
  created_at: string;
  avatar_url?: string;
}

export interface BleephubOrg {
  id: number;
  login: string;
  name: string;
  description: string;
  billing_email?: string;
  created_at: string;
  avatar_url?: string;
}

export interface BleephubTeam {
  id: number;
  slug: string;
  name: string;
  description: string;
  privacy: "secret" | "closed";
  organization?: { id: number; login: string };
  created_at: string;
}

export interface GithubTeamMember {
  id: number;
  login: string;
  avatar_url: string;
  type: string;
  role?: "member" | "maintainer" | "all";
}

export interface GithubTeamMembership {
  state: "active" | "pending";
  role: "member" | "maintainer";
  url: string;
}

export interface GithubTeamRepo {
  id: number;
  full_name: string;
  name: string;
  owner: { login: string; type: string };
  permissions?: Record<string, boolean>;
  role_name?: string;
}

export interface GithubDeployKey {
  id: number;
  key: string;
  title: string;
  url: string;
  verified: boolean;
  created_at: string;
  read_only: boolean;
}

export interface BleephubAuditEvent {
  id: number;
  actor_login: string;
  action: string;
  entity_type: string;
  entity_id: number | string;
  details: Record<string, unknown>;
  created_at: string;
}

export interface BleephubGistFile {
  filename?: string;
  content?: string;
  raw_url?: string;
  size?: number;
  type?: string;
  language?: string;
}

export interface BleephubGist {
  id: string;
  description: string;
  public: boolean;
  owner: { login: string; type: string; avatar_url?: string };
  files: Record<string, BleephubGistFile>;
  html_url?: string;
  created_at: string;
  updated_at: string;
  history?: GithubGistCommit[];
  forks?: BleephubGist[];
  forks_url?: string;
  commits_url?: string;
}

export interface GithubGistCommit {
  url: string;
  version: string;
  user: { login: string; type: string; avatar_url?: string } | null;
  change_status: Record<string, number>;
  committed_at: string;
}

export interface GithubNotificationThread {
  id: string;
  repository: Record<string, unknown>;
  subject: {
    title: string;
    url: string;
    latest_comment_url: string;
    type: string;
  };
  reason: string;
  unread: boolean;
  updated_at: string;
  last_read_at: string | null;
  subscription_url: string;
  url: string;
}

export interface GithubThreadSubscription {
  subscribed: boolean;
  ignored: boolean;
  reason: string;
  created_at: string;
  url: string;
  thread_url: string;
}

// ─── GitHub Discussions GraphQL shapes ──────────────────────────────────

export interface GithubDiscussionCategory {
  id: string;
  name: string;
  emoji: string;
  description: string;
  isAnswerable: boolean;
}

export interface GithubDiscussionAuthor {
  login: string;
  avatarUrl?: string;
}

export interface GithubDiscussion {
  id: string;
  number: number;
  title: string;
  body: string;
  bodyHTML: string;
  bodyText: string;
  author: GithubDiscussionAuthor | null;
  category: GithubDiscussionCategory;
  createdAt: string;
  updatedAt: string;
  comments: { totalCount: number };
}

export interface GithubDiscussionComment {
  id: string;
  databaseId: number;
  author: GithubDiscussionAuthor | null;
  body: string;
  bodyHTML: string;
  createdAt: string;
  updatedAt: string;
  isAnswer: boolean;
  replies: { nodes: GithubDiscussionComment[] };
}

export interface GithubDiscussionConnection {
  nodes: GithubDiscussion[];
  totalCount: number;
  pageInfo: {
    hasNextPage: boolean;
    endCursor: string | null;
  };
}

export interface GithubDiscussionCategoryConnection {
  nodes: GithubDiscussionCategory[];
  totalCount: number;
}

export interface GithubDiscussionCommentConnection {
  nodes: GithubDiscussionComment[];
  totalCount: number;
}


export interface GithubProjectClassic {
  id: number;
  node_id: string;
  name: string;
  body: string;
  state: "open" | "closed";
  number: number;
  creator: { login: string; avatar_url?: string } | null;
  created_at: string;
  updated_at: string;
  url: string;
  html_url: string;
  columns_url: string;
}

export interface GithubProjectColumn {
  id: number;
  node_id: string;
  name: string;
  created_at: string;
  updated_at: string;
  url: string;
  project_url: string;
  cards_url: string;
}

export interface GithubProjectCard {
  id: number;
  node_id: string;
  note: string | null;
  creator: { login: string; avatar_url?: string } | null;
  created_at: string;
  updated_at: string;
  url: string;
  column_url: string;
  project_url: string;
  content_url: string | null;
}

export interface GithubSecretScanningLocationDetails {
  path: string;
  start_line: number;
  end_line: number;
  start_column: number;
  end_column: number;
  blob_sha: string;
  blob_url: string;
  commit_sha: string;
  commit_url: string;
  html_url: string;
}

export interface GithubSecretScanningLocation {
  type: "commit";
  details: GithubSecretScanningLocationDetails;
}

export interface GithubSecretScanningAlert {
  number: number;
  state: "open" | "resolved";
  resolution: string | null;
  secret_type: string;
  secret_type_display_name: string;
  created_at: string;
  updated_at: string;
  resolved_at: string | null;
  url: string;
  html_url: string;
  locations_url: string;
}

export type GithubSecretScanningResolution =
  | "false_positive"
  | "wont_fix"
  | "revoked"
  | "used_in_tests"
  | "pattern_deleted"
  | "pattern_edited";

// ─── GitHub Code Scanning shapes ────────────────────────────────────────

export type GithubCodeScanningAlertState = "open" | "dismissed" | "fixed";

export type GithubCodeScanningDismissedReason =
  | "false_positive"
  | "won't_fix"
  | "used_in_tests"
  | "ignored";

export interface GithubCodeScanningAlertLocation {
  path: string;
  start_line: number;
  end_line: number;
  start_column: number;
  end_column: number;
}

export interface GithubCodeScanningAlertInstance {
  ref: string;
  analysis_key: string;
  category: string;
  state: GithubCodeScanningAlertState;
  commit_sha: string;
  message: { text: string };
  location: GithubCodeScanningAlertLocation;
}

export interface GithubCodeScanningAlert {
  number: number;
  state: GithubCodeScanningAlertState;
  created_at: string;
  updated_at: string;
  url: string;
  html_url: string;
  instances_url: string;
  fixed_at: string | null;
  dismissed_at: string | null;
  dismissed_reason: GithubCodeScanningDismissedReason | null;
  dismissed_comment: string | null;
  rule: {
    id: string;
    severity: string | null;
    description: string | null;
    name: string;
  };
  tool: { name: string | null };
  most_recent_instance: GithubCodeScanningAlertInstance | null;
}

export interface GithubCodeScanningAnalysis {
  id: number;
  ref: string;
  commit_sha: string;
  analysis_key: string;
  environment: string;
  category: string;
  error: string;
  created_at: string;
  results_count: number;
  rules_count: number;
  url: string;
  sarif_id: string;
  tool: { name: string | null };
  deletable: boolean;
  warning: string;
}

export interface GithubCodeScanningSARIFUpload {
  id: string;
  url: string;
}

export interface GithubCodeScanningSARIFStatus {
  processing_status: "pending" | "complete" | "failed";
  analyses_url: string | null;
  errors: string[] | null;
}

// ─── GitHub Dependabot shapes ───────────────────────────────────────────

export type GithubDependabotAlertState = "open" | "dismissed" | "fixed" | "auto_dismissed";

export type GithubDependabotDismissedReason =
  | "fix_started"
  | "inaccurate"
  | "no_bandwidth"
  | "not_used"
  | "tolerable_risk";

export interface GithubDependabotAlertPackage {
  ecosystem: string;
  name: string;
}

export interface GithubDependabotAlert {
  number: number;
  state: GithubDependabotAlertState;
  dependency: {
    package: GithubDependabotAlertPackage;
    manifest_path: string;
  };
  security_advisory: {
    ghsa_id: string;
    cve_id: string | null;
    summary: string;
    description: string;
    severity: string;
  };
  security_vulnerability: {
    package: GithubDependabotAlertPackage;
    severity: string;
    vulnerable_version_range: string;
    first_patched_version: { identifier: string } | null;
  };
  url: string;
  html_url: string;
  created_at: string;
  updated_at: string;
  dismissed_at: string | null;
  dismissed_by: { login: string } | null;
  dismissed_reason: GithubDependabotDismissedReason | null;
  dismissed_comment: string | null;
  fixed_at: string | null;
  auto_dismissed_at: string | null;
}

export interface GithubDependabotSecret {
  name: string;
  created_at: string;
  updated_at: string;
  visibility?: GithubOrgVisibility;
}

// ─── GitHub Codespaces shapes ───────────────────────────────────────────

export type GithubCodespaceState =
  | "Available"
  | "Shutdown"
  | "Creating"
  | "Unavailable";

export interface GithubCodespaceMachine {
  name: string;
  display_name: string;
  operating_system: string;
  storage_in_bytes: number;
  memory_in_bytes: number;
  cpus: number;
  prebuild_availability: string;
}

export interface GithubCodespace {
  id: number;
  name: string;
  display_name: string;
  environment_id: string;
  owner: { login: string; type: string; avatar_url?: string } | null;
  billable_owner: { login: string; type: string; avatar_url?: string } | null;
  repository: { id: number; full_name: string; name: string; owner: { login: string; type: string } } | null;
  machine: GithubCodespaceMachine;
  created_at: string;
  updated_at: string;
  last_used_at: string;
  state: GithubCodespaceState;
  url: string;
  html_url: string;
  web_url: string;
  billing_url: string;
  git_status: { ahead: number; behind: number; has_uncommitted_changes: boolean; ref: string };
  devcontainer_path: string;
  image: string;
  retention_period_minutes: number;
}

export interface GithubCodespaceSecret {
  name: string;
  key: string;
  created_at: string;
  updated_at: string;
  visibility?: GithubOrgVisibility;
}

export interface CodespaceCreatePayload {
  repository_id?: number;
  ref?: string;
  machine?: string;
  display_name?: string;
  location?: string;
}

// ─── GitHub Packages REST shapes ────────────────────────────────────────

export type GithubPackageType = "npm" | "maven" | "rubygems" | "nuget" | "docker" | "container";
export type GithubPackageVisibility = "public" | "private" | "internal";

export interface GithubPackage {
  id: number;
  node_id: string;
  name: string;
  package_type: GithubPackageType;
  visibility: GithubPackageVisibility;
  url: string;
  html_url: string;
  version_count: number;
  created_at: string;
  updated_at: string;
  owner: { login: string; type: string; avatar_url?: string } | null;
  repository: BleephubRepo | null;
}

export interface GithubPackageVersion {
  id: number;
  node_id: string;
  name: string;
  url: string;
  package_html_url: string;
  html_url: string;
  license: string | null;
  description: string;
  created_at: string;
  updated_at: string;
  deleted_at?: string;
  metadata: {
    package_type: GithubPackageType;
    container?: { tags: string[] };
    docker?: { tag: string[] };
  };
}

export interface GithubPackageFile {
  id: number;
  node_id: string;
  name: string;
  content_type: string;
  size: number;
  url: string;
  html_url: string;
  download_url: string;
}

export interface GithubPackageVersionCreatePayload {
  version: string;
  description?: string;
  metadata?: GithubPackageVersion["metadata"];
  files: { name: string; content_type: string; content_base64: string }[];
}

// ─── GitHub Security Advisories shapes ──────────────────────────────────

export type GithubSecurityAdvisorySeverity = "critical" | "high" | "medium" | "low";
export type GithubSecurityAdvisoryState = "draft" | "published" | "closed";

export interface GithubSecurityAdvisory {
  id: number;
  ghsa_id: string;
  cve_id: string | null;
  summary: string;
  description: string;
  severity: GithubSecurityAdvisorySeverity;
  cwe_ids?: string[];
  state: GithubSecurityAdvisoryState;
  author: { login: string } | null;
  created_at: string;
  updated_at: string;
  published_at: string | null;
  url: string;
  html_url: string;
}

export interface GithubSecurityAdvisoryCreatePayload {
  summary: string;
  description: string;
  severity: GithubSecurityAdvisorySeverity;
  cwe_ids?: string[];
}

export interface GithubVulnerabilityReportPayload {
  summary: string;
  description: string;
  severity?: GithubSecurityAdvisorySeverity;
  cwe_ids?: string[];
}

// ─── GitHub Repository Rulesets shapes ──────────────────────────────────

export type GithubRulesetTarget = "branch" | "tag";
export type GithubRulesetEnforcement = "disabled" | "active" | "evaluate";

export interface GithubRuleset {
  id: number;
  name: string;
  target: GithubRulesetTarget;
  source_type: "Repository" | "Organization";
  source: string;
  enforcement: GithubRulesetEnforcement;
  bypass_actors?: Array<{
    actor_id: number;
    actor_type: string;
    bypass_mode: string;
  }>;
  conditions?: Record<string, unknown>;
  rules?: Array<{
    type: string;
    parameters?: Record<string, unknown>;
  }>;
  created_at?: string;
  updated_at?: string;
}

export interface GithubRulesetCreatePayload {
  name: string;
  target: GithubRulesetTarget;
  enforcement: GithubRulesetEnforcement;
  rules?: Array<{ type: string; parameters?: Record<string, unknown> }>;
  conditions?: Record<string, unknown>;
  bypass_actors?: Array<{
    actor_id: number;
    actor_type: string;
    bypass_mode: string;
  }>;
}
