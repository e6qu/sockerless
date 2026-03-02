import type {
  GitlabhubPipeline,
  GitlabhubRunner,
  GitlabhubProject,
  GitlabhubMetrics,
  GitlabhubStatus,
  GitlabhubHealth,
} from "./types.js";

async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json() as Promise<T>;
}

export const fetchPipelines = () =>
  fetchJSON<GitlabhubPipeline[]>("/internal/pipelines");

export const fetchPipelineDetail = (id: number) =>
  fetchJSON<GitlabhubPipeline>(`/internal/pipelines/${id}`);

export const fetchPipelineLogs = (id: number) =>
  fetchJSON<Record<string, string[]>>(`/internal/pipelines/${id}/logs`);

export const fetchRunners = () =>
  fetchJSON<GitlabhubRunner[]>("/internal/runners");

export const fetchProjects = () =>
  fetchJSON<GitlabhubProject[]>("/internal/projects");

export const fetchMetrics = () =>
  fetchJSON<GitlabhubMetrics>("/internal/metrics");

export const fetchStatus = () =>
  fetchJSON<GitlabhubStatus>("/internal/status");

export const fetchHealth = () =>
  fetchJSON<GitlabhubHealth>("/health");
