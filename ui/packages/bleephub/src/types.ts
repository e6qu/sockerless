/** Workflow represents a running multi-job workflow. */
export interface BleephubWorkflow {
  id: string;
  name: string;
  runId: number;
  runNumber: number;
  jobs: Record<string, BleephubWorkflowJob>;
  status: string; // "running" | "completed" | "pending_concurrency"
  result: string; // "success" | "failure" | "cancelled"
  createdAt: string;
  eventName?: string;
  ref?: string;
  sha?: string;
  repoFullName?: string;
  concurrencyGroup?: string;
}

/** WorkflowJob represents a single job within a workflow. */
export interface BleephubWorkflowJob {
  key: string;
  jobId: string;
  displayName: string;
  needs?: string[];
  status: string; // "pending" | "queued" | "running" | "completed" | "skipped"
  result: string; // "success" | "failure" | "cancelled" | "skipped"
  matrix?: Record<string, unknown>;
  continueOnError?: boolean;
  startedAt?: string;
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
  ephemeral?: boolean;
  createdOn: string;
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
