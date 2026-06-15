// Wire types mirroring bleeplab's /internal read API DTOs (internal_api.go).
// Field names match the Go json tags exactly.

export interface Status {
  projects: number;
  pipelines: number;
  pipelines_by_status: Record<string, number>;
  jobs: number;
  jobs_by_status: Record<string, number>;
  connected_runners: number;
  uptime_seconds: number;
}

export interface Project {
  id: number;
  name: string;
  path: string;
  default_branch: string;
  sha: string;
  pipelines: number;
}

export interface Pipeline {
  id: number;
  project_id: number;
  project_name: string;
  ref: string;
  sha: string;
  status: string;
  stages: string[];
  jobs: number;
}

export interface Job {
  id: number;
  name: string;
  stage: string;
  status: string;
  ref: string;
  sha: string;
  project_id: number;
  artifact_size: number;
  artifact_filename?: string;
  trace?: string;
}

export interface PipelineDetail extends Pipeline {
  job_list: Job[];
}

export interface Runner {
  id: number;
  token: string;
}

export interface BackendInfo {
  backend: string;
  detail: string;
}

export interface Storage {
  git: BackendInfo;
  artifacts: BackendInfo;
}
