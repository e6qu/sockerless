// Enum unions mirror the exact strings the server emits (bleephub Go:
// workflows.go / store_workflow_files.go). Empty result = still in flight.
// Keeping these as unions makes a typo'd comparison (e.g. "failed" vs the
// real "failure") a compile error rather than a silently-dead branch.
// Only values the server actually ASSIGNS belong here — a workflow is never
// "queued"/"skipped", a workflow file is never anything but "active".
// "waiting" = held on a reviewer-protected environment approval.
export type WorkflowStatus =
  | "queued"
  | "in_progress"
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
export type JobResult =
  | "success"
  | "failure"
  | "cancelled"
  | "skipped"
  | "neutral"
  | "timed_out"
  | "action_required";
export type WorkflowResult = "" | JobResult;
export type WorkflowFileState = "active" | "disabled_manually" | "disabled_inactivity";
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
  ssh_url?: string;
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

/** Dashboard metrics derived from public GitHub REST repository and Actions routes. */
export interface BleephubMetrics {
  workflow_runs: number;
  job_dispatches: number;
  jobs_by_status: Record<string, number>;
  job_completions: Record<string, number>;
  active_workflows: number;
  connected_runners: number;
}

/** Runtime status derived from public GitHub REST repository and Actions routes. */
export interface BleephubStatus {
  active_workflows: number;
  jobs_by_status: Record<string, number>;
  connected_runners: number;
}

/** Health response from /health. */
export interface BleephubHealth {
  status: string;
  service: string;
  enterprise_slug: string;
}

/** WorkflowFile is the file-level workflow YAML entity. */
export interface BleephubWorkflowFile {
  id: number;
  name: string;
  path: string;
  state: WorkflowFileState;
  repoFullName: string;
  source?: WorkflowFileSource;
  createdAt: string;
  updatedAt: string;
}

/** Body for POST /api/v3/repos/{o}/{r}/actions/workflows/{id}/dispatches. */
export interface BleephubDispatchRequest {
  ref?: string;
  inputs?: Record<string, string>;
}

/** GitHub App row from the settings-owned browser surface. */
export interface BleephubApp {
  id: number;
  slug: string;
  name: string;
  description: string;
  ownerId: number;
  createdAt: string;
}

/** GitHub App installation row normalized from GitHub's REST installation shape. */
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

/** OAuth App row from the settings-owned browser surface, distinct from GitHub App. */
export interface BleephubOAuthApp {
  clientId: string;
  name: string;
  description: string;
  url: string;
  callbackUrl: string;
  ownerId: number;
  createdAt: string;
}

export interface WireGitHubApp {
  id: number;
  slug: string;
  name: string;
  description: string;
  owner: { id: number };
  created_at: string;
}

export interface WireInstallation {
  id: number;
  app_id: number;
  app_slug: string;
  target_type: string;
  repository_selection: string;
  created_at: string;
  suspended_at: string | null;
  account: { login: string };
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
  /** null when the issue is not in a milestone. */
  milestone?: GithubMilestone | null;
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
  created_at: string;
  updated_at: string;
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
  node_id: string;
  tag_name: string;
  target_commitish: string;
  name: string;
  body: string;
  draft: boolean;
  prerelease: boolean;
  created_at: string;
  /** null until the release is published (drafts). */
  published_at: string | null;
  html_url: string;
  url: string;
  assets_url: string;
  upload_url: string;
  assets: GithubReleaseAsset[];
}

export interface GithubReleaseAsset {
  id: number;
  node_id: string;
  name: string;
  label: string;
  state: "uploaded";
  content_type: string;
  size: number;
  download_count: number;
  created_at: string;
  updated_at: string;
  url: string;
  browser_download_url: string;
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

export interface GithubCodeQLDatabase {
  id: number;
  name: string;
  language: string;
  uploader: { login: string; type: string; avatar_url?: string } | null;
  content_type: string;
  size: number;
  created_at: string;
  updated_at: string;
  url: string;
  commit_oid: string | null;
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

// ─── Repo insights ───────────────────────────────────────────────────────

/** One entry of GET /repos/{o}/{r}/contributors — a resolved account or an
 * anonymous git author (type "Anonymous", identified by name/email). */
export interface GithubContributor {
  login?: string;
  avatar_url?: string;
  type: string;
  contributions: number;
  name?: string;
  email?: string;
}

export interface GithubTrafficBucket {
  timestamp: string;
  count: number;
  uniques: number;
}

export interface GithubTrafficViews {
  count: number;
  uniques: number;
  views: GithubTrafficBucket[];
}

export interface GithubTrafficClones {
  count: number;
  uniques: number;
  clones: GithubTrafficBucket[];
}

export interface GithubTrafficPath {
  path: string;
  title: string;
  count: number;
  uniques: number;
}

export interface GithubTrafficReferrer {
  referrer: string;
  count: number;
  uniques: number;
}

/** One week of GET /repos/{o}/{r}/stats/commit_activity: Unix week start,
 * per-weekday commit counts (Sunday first), and the week total. */
export interface GithubCommitActivityWeek {
  week: number;
  days: number[];
  total: number;
}

export interface GithubCommunityFile {
  url: string;
  html_url: string;
}

export interface GithubCommunityProfile {
  health_percentage: number;
  description: string | null;
  documentation: string | null;
  files: {
    code_of_conduct: { key: string; name: string } | null;
    code_of_conduct_file: GithubCommunityFile | null;
    license: { key: string; name: string; spdx_id: string } | null;
    contributing: GithubCommunityFile | null;
    readme: GithubCommunityFile | null;
    issue_template: GithubCommunityFile | null;
    pull_request_template: GithubCommunityFile | null;
  };
  updated_at: string;
}

// ─── Labels + milestones ────────────────────────────────────────────────

export interface GithubLabel {
  id: number;
  name: string;
  color: string;
  description: string;
  default: boolean;
}

export interface GithubMilestone {
  id: number;
  number: number;
  title: string;
  description: string;
  state: "open" | "closed";
  creator: { login: string; avatar_url: string } | null;
  open_issues: number;
  closed_issues: number;
  due_on: string | null;
  closed_at: string | null;
  created_at: string;
  updated_at: string;
}

// ─── Organization governance ────────────────────────────────────────────

/** GitHub `simple-user` — the shape people-management lists return. */
export interface GithubAccount {
  id: number;
  login: string;
  avatar_url: string;
  type: string;
  site_admin: boolean;
}

export interface GithubOrgInvitation {
  id: number;
  login: string | null;
  email: string | null;
  role: string;
  created_at: string;
  failed_at: string | null;
  failed_reason: string | null;
  inviter: GithubAccount | null;
  team_count: number;
  invitation_source: string;
}

export type GithubCustomPropertyValueType =
  | "string"
  | "single_select"
  | "multi_select"
  | "true_false"
  | "url";

export interface GithubCustomProperty {
  property_name: string;
  value_type: GithubCustomPropertyValueType;
  required: boolean;
  default_value: unknown;
  description: string | null;
  allowed_values?: string[];
  values_editable_by: string;
  require_explicit_values: boolean;
}

export interface GithubIssueType {
  id: number;
  node_id: string;
  name: string;
  description: string | null;
  color: string | null;
  is_enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface GithubOrgRole {
  id: number;
  name: string;
  description: string;
  base_role: string;
  source: string;
  permissions: string[];
  created_at: string;
  updated_at: string;
}

/** Team assigned an organization role (team-simple + assignment). */
export interface GithubOrgRoleTeam {
  id: number;
  slug: string;
  name: string;
  description: string | null;
  assignment: string;
}

/** User assigned an organization role (simple-user + assignment). */
export interface GithubOrgRoleUser extends GithubAccount {
  assignment: string;
}

// ─── Enterprise administration ──────────────────────────────────────────

export interface GithubEnterpriseTeam {
  id: number;
  name: string;
  slug: string;
  description: string | null;
  organization_selection_type: string;
  group_id: string | null;
  notification_setting: string;
  created_at: string;
  updated_at: string;
}

export interface GithubEnterpriseDependabotAccess {
  default_level: string | null;
  accessible_repositories: BleephubRepo[];
}

// ─── Copilot ────────────────────────────────────────────────────────────

export interface GithubCopilotSeatBreakdown {
  total: number;
  added_this_cycle: number;
  pending_cancellation: number;
  pending_invitation: number;
  active_this_cycle: number;
  inactive_this_cycle: number;
}

export interface GithubCopilotBilling {
  seat_breakdown: GithubCopilotSeatBreakdown;
  public_code_suggestions: string;
  ide_chat: string;
  platform_chat: string;
  cli: string;
  seat_management_setting: string;
  plan_type: string;
}

export interface GithubCopilotSeat {
  assignee: GithubAccount | null;
  assigning_team: { slug: string; name: string } | null;
  pending_cancellation_date: string | null;
  last_activity_at: string | null;
  last_activity_editor: string | null;
  created_at: string;
  updated_at: string;
  plan_type: string;
}

export interface GithubCopilotSpace {
  id: number;
  number: number;
  name: string;
  description: string | null;
  general_instructions: string | null;
  base_role: string;
  owner: { login: string } | null;
  creator: GithubAccount | null;
  created_at: string;
  updated_at: string;
}

// ─── Copilot Spaces CRUD · enterprise team orgs · custom property values ─

/** Copilot Space collaborator: `simple-user` or team-simple plus actor_type + role. */
export interface GithubCopilotSpaceCollaborator {
  actor_type: "User" | "Team";
  role: string;
  id: number;
  /** Present when actor_type is "User". */
  login?: string;
  /** Present when actor_type is "Team". */
  slug?: string;
  name?: string;
}

export interface GithubCopilotSpaceResource {
  id: number;
  resource_type: string;
  metadata: Record<string, unknown>;
}

// ─── Deployments + webhook deliveries + Pages ───────────────────────────

/** Deployment — GET /repos/{o}/{r}/deployments (items). */
export interface GithubDeployment {
  id: number;
  node_id: string;
  sha: string;
  ref: string;
  task: string;
  environment: string;
  original_environment: string;
  description: string;
  creator: { login: string } | null;
  payload: Record<string, unknown> | null;
  created_at: string;
  updated_at: string;
  transient_environment: boolean;
  production_environment: boolean;
  statuses_url: string;
}

/** Deployment status state (the POST statuses `state` enum). */
export type GithubDeploymentState =
  | "error"
  | "failure"
  | "inactive"
  | "in_progress"
  | "queued"
  | "pending"
  | "success";

/** Deployment status — GET .../deployments/{id}/statuses (items). */
export interface GithubDeploymentStatus {
  id: number;
  node_id: string;
  state: GithubDeploymentState;
  creator: { login: string } | null;
  description: string;
  environment: string;
  target_url: string;
  log_url: string;
  environment_url: string;
  created_at: string;
  updated_at: string;
}

// ─── PR reviews, statuses, reactions & timeline ─────────────────────────

export type GithubReviewState =
  | "APPROVED"
  | "CHANGES_REQUESTED"
  | "COMMENTED"
  | "DISMISSED"
  | "PENDING";

/** Pull request review — GET .../pulls/{n}/reviews (items). */
export interface GithubPRReview {
  id: number;
  /** null when the authoring user no longer resolves (GitHub parity). */
  user: { login: string; avatar_url: string } | null;
  body: string;
  state: GithubReviewState;
  commit_id: string;
  submitted_at: string | null;
}

/** Pull request review comment — GET .../pulls/{n}/comments (items). */
export interface GithubPRReviewComment {
  id: number;
  pull_request_review_id: number;
  in_reply_to_id?: number;
  diff_hunk: string;
  path: string;
  line: number | null;
  side: string;
  body: string;
  /** null when the authoring user no longer resolves (GitHub parity). */
  user: { login: string; avatar_url: string } | null;
  created_at: string;
  updated_at: string;
}

/** GitHub `organization-simple` — enterprise team organization assignments. */
export interface GithubOrgSimple {
  id: number;
  login: string;
  avatar_url: string;
  description: string | null;
}

/** One repository row of GET /orgs/{org}/properties/values. */
export interface GithubOrgRepoCustomPropertyValues {
  repository_id: number;
  repository_name: string;
  repository_full_name: string;
  properties: { property_name: string; value: unknown }[];
}

/** Wait-timer / required-reviewers rule nested in the environment object. */
export interface GithubEnvironmentProtectionRule {
  id: number;
  node_id: string;
  type: string;
  wait_timer?: number;
  reviewers?: { type: string; reviewer?: { login?: string } }[];
}

/**
 * Full environment — GET /repos/{o}/{r}/environments (items). Carries the
 * protection config the slim GithubEnvironment omits.
 */
export interface GithubEnvironmentDetail {
  id: number;
  node_id: string;
  name: string;
  url: string;
  html_url: string;
  created_at: string;
  updated_at: string;
  deployment_branch_policy: {
    protected_branches: boolean;
    custom_branch_policies: boolean;
  } | null;
  protection_rules: GithubEnvironmentProtectionRule[];
}

/** Branch/tag pattern — GET .../environments/{env}/deployment-branch-policies. */
export interface GithubDeploymentBranchPolicy {
  id: number;
  node_id: string;
  name: string;
  type: "branch" | "tag";
}

/** Custom (app-backed) rule — GET .../environments/{env}/deployment_protection_rules. */
export interface GithubEnvCustomProtectionRule {
  id: number;
  node_id: string;
  enabled: boolean;
  app: { id: number; slug: string; integration_url: string; node_id: string } | null;
}

/** Delivery summary — GET {hook}/deliveries (items). */
export interface GithubHookDelivery {
  id: number;
  guid: string;
  delivered_at: string;
  redelivery: boolean;
  duration: number;
  status: string;
  status_code: number;
  event: string;
  action: string | null;
  installation_id: number | null;
  repository_id: number | null;
  throttled_at: string | null;
}

/** Full delivery — GET {hook}/deliveries/{id} (adds request/response). */
export interface GithubHookDeliveryDetail extends GithubHookDelivery {
  url: string;
  request: { headers: Record<string, string> | null; payload: unknown };
  response: { headers: Record<string, string> | null; payload: string | null };
}

/** Organization webhook — GET /orgs/{org}/hooks (items). */
export interface GithubOrgWebhook {
  id: number;
  type: string;
  name: string;
  active: boolean;
  events: string[];
  config: { url: string; content_type: string; insecure_ssl: string };
  created_at: string;
  updated_at: string;
  url: string;
  ping_url: string;
  deliveries_url: string;
}

/** Pages site — GET /repos/{o}/{r}/pages. */
export interface GithubPagesSite {
  cname: string;
  url: string;
  html_url: string;
  status: string;
  source: { branch?: string; path?: string } | null;
  public: boolean;
  custom_404: boolean;
  protected_domain_state: string | null;
  build_type: string | null;
  https_enforced: boolean;
}

/** Pages build — GET /repos/{o}/{r}/pages/builds (items). */
export interface GithubPagesBuild {
  url: string;
  status: string;
  pusher: { login: string; id: number; type: string } | null;
  commit: string;
  created_at: string;
  updated_at: string;
  duration: number;
  error: { message: string | null } | null;
}

/** One domain's checks inside the Pages health-check response. */
export interface GithubPagesDomainHealth {
  host: string;
  uri: string;
  nameservers: string;
  dns_resolves: boolean;
  is_valid_domain: boolean;
  is_apex_domain: boolean;
  is_pages_domain: boolean;
  is_valid: boolean;
  reason: string | null;
  enforces_https: boolean;
}

/** Pages health check — GET /repos/{o}/{r}/pages/health. */
export interface GithubPagesHealth {
  domain: GithubPagesDomainHealth | null;
  alt_domain: GithubPagesDomainHealth | null;
}

/** Pull request review thread — GraphQL PullRequest.reviewThreads node. */
export interface GithubPRReviewThread {
  id: string;
  isResolved: boolean;
  comments: { databaseId: number }[];
}

/** Requested reviewers — GET .../pulls/{n}/requested_reviewers. */
export interface GithubReviewRequest {
  users: GithubAccount[];
  teams: { id: number; slug: string; name: string }[];
}

export type GithubCommitStatusState = "success" | "failure" | "error" | "pending";

/** One status context inside the combined commit status. */
export interface GithubCommitStatus {
  context: string;
  state: GithubCommitStatusState;
  description: string | null;
  target_url: string | null;
}

/** Combined commit status — GET .../commits/{ref}/status. */
export interface GithubCombinedStatus {
  state: GithubCommitStatusState;
  sha: string;
  total_count: number;
  statuses: GithubCommitStatus[];
}

export type GithubReactionContent =
  | "+1"
  | "-1"
  | "laugh"
  | "confused"
  | "heart"
  | "hooray"
  | "rocket"
  | "eyes";

/** Reaction — GET .../reactions (items). */
export interface GithubReaction {
  id: number;
  content: GithubReactionContent;
  /** null when the reacting user no longer resolves (GitHub parity). */
  user: { login: string } | null;
  created_at: string;
}

/**
 * Issue-timeline union member — GET .../issues/{n}/timeline (items).
 * Only `event` is guaranteed; every other member depends on the event
 * variant (commented, reviewed, labeled, assigned, renamed, …).
 */
export interface GithubTimelineItem {
  event: string;
  id?: number;
  actor?: { login: string; avatar_url: string } | null;
  user?: { login: string; avatar_url: string } | null;
  body?: string;
  created_at?: string;
  submitted_at?: string | null;
  state?: string;
  label?: { name: string; color: string } | null;
  assignee?: { login: string } | null;
  rename?: { from: string; to: string } | null;
  commit_id?: string | null;
}

// ─── Search + repo social + account ─────────────────────────────────────

/** Item from GET /search/issues: issue shape plus PR marker + repository. */
export interface GithubSearchIssueItem {
  id: number;
  number: number;
  title: string;
  state: string;
  user: { login: string } | null;
  comments: number;
  created_at: string;
  updated_at: string;
  /** null for plain issues; set when the result is a pull request. */
  pull_request: { url: string } | null;
  repository: { full_name: string };
}

/** Item from GET /search/code. */
export interface GithubSearchCodeItem {
  name: string;
  path: string;
  sha: string;
  html_url: string;
  language: string | null;
  repository: { full_name: string };
}

/** Item from GET /search/users (users and orgs share the shape). */
export interface GithubSearchUserItem {
  id: number;
  login: string;
  type: string;
  name?: string | null;
  bio?: string | null;
}

/** Item from GET /search/commits. */
export interface GithubSearchCommitItem {
  sha: string;
  commit: {
    message: string;
    author: { name: string; email: string; date: string };
  };
  author: { login: string } | null;
  repository: { full_name: string };
}

/** Item from GET /search/labels. */
export interface GithubSearchLabelItem {
  id: number;
  name: string;
  color: string;
  default: boolean;
  description: string | null;
}

/** Item from GET /search/topics. */
export interface GithubSearchTopicItem {
  name: string;
  repository_count: number;
  created_at: string;
  updated_at: string;
}

/** Repo collaborator: simple user plus permission grants. */
export interface GithubCollaborator {
  id: number;
  login: string;
  type: string;
  role_name: string;
  permissions: { pull: boolean; push: boolean; admin: boolean };
}

/** Pending repository invitation. */
export interface GithubRepoInvitation {
  id: number;
  invitee: { login: string } | null;
  inviter: { login: string } | null;
  permissions: string;
  created_at: string;
  expired: boolean;
}

/** Git tag with source-archive download links. */
export interface GithubTag {
  name: string;
  zipball_url: string;
  tarball_url: string;
  commit: { sha: string; url: string };
}

/** Social counters served on the full-repository shape. */
export interface GithubRepoSocialCounts {
  stargazers_count: number;
  subscribers_count: number;
  forks_count: number;
}

/** Authenticated viewer's repository notification preference. */
export interface GithubRepoSubscription {
  subscribed: boolean;
  ignored: boolean;
  reason: string | null;
  created_at: string;
  url: string;
  repository_url: string;
}

/** Viewer-specific state normalized for repository page rendering. */
export interface GithubRepoViewerState {
  starred: boolean;
  subscribed: boolean;
}

/** Email address on the authenticated user's account. */
export interface GithubUserEmail {
  email: string;
  primary: boolean;
  verified: boolean;
  visibility: string | null;
}

/** SSH authentication key on the authenticated user's account. */
export interface GithubSSHKey {
  id: number;
  key: string;
  title: string;
  verified: boolean;
  created_at: string;
  read_only: boolean;
}

export interface GithubGPGKeyEmail {
  email: string;
  verified: boolean;
  primary: boolean;
}

/** GPG key on the authenticated user's account. */
export interface GithubGPGKey {
  id: number;
  key_id: string;
  public_key: string;
  can_sign: boolean;
  can_encrypt_commits: boolean;
  can_certify: boolean;
  created_at: string;
  name?: string;
  emails?: GithubGPGKeyEmail[];
  expires_at?: string;
}

/** SSH signing key on the authenticated user's account. */
export interface GithubSSHSigningKey {
  id: number;
  key: string;
  title: string;
  created_at: string;
}

/** Entry in GET /user/blocks. */
export interface GithubBlockedUser {
  login: string;
}

// ─── Bleephub dashboard, user profile, and organization pages ───────────

/**
 * The GitHub `public-user` shape served by GET /users/{login} and
 * GET /user — the simple-user members plus the profile fields and live
 * counters (followers/following/public_repos). company/location/
 * twitter_username are null when unset, matching real GitHub.
 */
export interface GithubUserProfile {
  login: string;
  id: number;
  avatar_url: string;
  type: string;
  site_admin: boolean;
  name: string | null;
  email: string | null;
  bio: string | null;
  blog: string | null;
  company: string | null;
  location: string | null;
  twitter_username: string | null;
  followers: number;
  following: number;
  public_repos: number;
  created_at: string;
  html_url?: string;
}

/**
 * The GitHub `organization-full` shape served by GET /orgs/{org} —
 * profile fields plus the live public_repos counter.
 */
export interface GithubOrgProfile {
  login: string;
  id: number;
  avatar_url: string;
  description: string | null;
  name: string | null;
  company: string | null;
  blog: string | null;
  location: string | null;
  email: string | null;
  twitter_username: string | null;
  public_repos: number;
  followers: number;
  following: number;
  html_url: string;
  created_at: string;
}

/** The GitHub `organization-simple` shape in GET /users/{login}/orgs. */
export interface GithubOrgSummary {
  login: string;
  id: number;
  avatar_url: string;
  description: string | null;
}

/** The GitHub `team` (team-simple) shape in GET /orgs/{org}/teams. */
export interface GithubOrgTeam {
  id: number;
  slug: string;
  name: string;
  description: string | null;
  privacy: string;
  permission: string;
  html_url: string;
  parent: { slug: string; name: string } | null;
}

/**
 * An issue from GET /issues (the authenticated user's cross-repo issue
 * feed). Unlike the repo-scoped issue shape it carries the `repository`
 * each result lives in, since results span repositories.
 */
export interface GithubFeedIssue {
  id: number;
  number: number;
  title: string;
  state: GithubState;
  comments: number;
  updated_at: string;
  html_url: string;
  repository: { full_name: string; name: string; owner: { login: string } };
}

// ─── GitHub Marketplace browser workflow ──────────────────────────────

export interface GithubMarketplacePlan {
  url: string;
  accounts_url: string;
  id: number;
  number: number;
  name: string;
  description: string;
  monthly_price_in_cents: number;
  yearly_price_in_cents: number;
  price_model: "FREE" | "FLAT_RATE" | "PER_UNIT";
  has_free_trial: boolean;
  unit_name: string | null;
  state: "draft" | "published";
  bullets: string[];
}

export interface GithubMarketplaceListing {
  slug: string;
  name: string;
  description: string;
  full_description: string;
  setup_url: string | null;
  installation_url: string | null;
  github_app_id: number | null;
  oauth_app_client_id: string | null;
  published: boolean;
  created_at: string;
  updated_at: string;
  plans: GithubMarketplacePlan[];
}

export interface GithubMarketplaceListingSettings extends GithubMarketplaceListing {
  webhook_url: string | null;
  webhook_content_type: "json" | "form";
  webhook_active: boolean;
  webhook_id: number | null;
}

export interface GithubMarketplaceAccount {
  id: number;
  login: string;
  type: "User" | "Organization";
  avatar_url: string;
}

export interface GithubMarketplacePendingChange {
  effective_date: string;
  billing_cycle: string | null;
  unit_count: number | null;
  cancellation: boolean;
  plan?: GithubMarketplacePlan;
}

export interface GithubMarketplaceSubscription {
  id: number;
  login: string;
  type: "User" | "Organization";
  marketplace_pending_change: GithubMarketplacePendingChange | null;
  marketplace_purchase: {
    billing_cycle: "monthly" | "yearly";
    next_billing_date: string | null;
    is_installed: boolean;
    unit_count: number | null;
    on_free_trial: boolean;
    free_trial_ends_on: string | null;
    updated_at: string | null;
    plan: GithubMarketplacePlan;
  };
  listing: GithubMarketplaceListing;
  account_login: string;
  setup_url: string | null;
}

// ─── WP-C: Issues & Pull Requests GitHub-faithful layout ────────────────

/** A changed file in a pull request — GET /pulls/{n}/files (items). */
export interface GithubPRFile {
  sha: string;
  filename: string;
  status: "added" | "removed" | "modified" | "renamed" | "changed" | "copied" | "unchanged";
  additions: number;
  deletions: number;
  changes: number;
  /** Unified-diff hunk text; absent for binary files. */
  patch?: string;
  previous_filename?: string;
  blob_url?: string;
}

/** Client-side list filters shared by the Issues and Pull Requests list bars. */
export interface ListFilterState {
  label: string | null;
  author: string | null;
  assignee: string | null;
  milestone: string | null;
  sort: "newest" | "oldest" | "comments" | "updated";
}
