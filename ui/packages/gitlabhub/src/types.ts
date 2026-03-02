/** Pipeline represents a GitLab CI pipeline. */
export interface GitlabhubPipeline {
  id: number;
  project_id: number;
  project_name: string;
  status: string; // "created" | "pending" | "running" | "success" | "failed" | "canceled"
  result: string;
  ref: string;
  sha: string;
  stages: string[];
  jobs: Record<string, GitlabhubPipelineJob>;
  created_at: string;
}

/** PipelineJob represents a job within a pipeline. */
export interface GitlabhubPipelineJob {
  id: number;
  pipeline_id: number;
  name: string;
  stage: string;
  status: string; // "created" | "pending" | "running" | "success" | "failed" | "canceled" | "skipped"
  result: string;
  allow_failure: boolean;
  when: string; // "on_success" | "always" | "never" | "manual"
  needs?: string[];
  started_at?: string;
  retry_count?: number;
  max_retries?: number;
  timeout?: number;
  matrix_group?: string;
  resource_group?: string;
}

/** Runner represents a registered GitLab runner. */
export interface GitlabhubRunner {
  id: number;
  description: string;
  active: boolean;
  tag_list: string[];
}

/** Project represents a GitLab project. */
export interface GitlabhubProject {
  id: number;
  name: string;
}

/** MetricsSnapshot is a point-in-time metrics report. */
export interface GitlabhubMetrics {
  pipeline_submissions: number;
  job_dispatches: number;
  job_completions: Record<string, number>;
  active_pipelines: number;
  registered_runners: number;
  uptime_seconds: number;
  goroutines: number;
  heap_alloc_mb: number;
}

/** Status response from /internal/status. */
export interface GitlabhubStatus {
  active_pipelines: number;
  jobs_by_status: Record<string, number>;
  registered_runners: number;
  uptime_seconds: number;
}

/** Health response from /health. */
export interface GitlabhubHealth {
  status: string;
  service: string;
}
